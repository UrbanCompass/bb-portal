import {
  BarChartOutlined,
  ClockCircleOutlined,
  DatabaseOutlined,
  ThunderboltOutlined,
  WarningOutlined,
} from "@ant-design/icons";
import { useQuery as useApolloQuery } from "@apollo/client/react";
import { useQuery } from "@tanstack/react-query";
import { Link, Navigate } from "@tanstack/react-router";
import {
  Alert,
  Col,
  List,
  Row,
  Skeleton,
  Space,
  Statistic,
  Typography,
} from "antd";
import dayjs from "dayjs";
import type React from "react";
import { CacheStatsPanel } from "@/components/CacheStatsPanel";
import { ExecutionRatioDonut } from "@/components/ExecutionRatioDonut";
import { InvocationResultTag } from "@/components/InvocationResultTag";
import PortalAlert from "@/components/PortalAlert";
import { PortalCard } from "@/components/PortalCard";
import Uploader from "@/components/Uploader";
import { gql } from "@/graphql/__generated__";
import {
  BazelInvocationOrderField,
  OrderDirection,
  type RunnerCount,
} from "@/graphql/__generated__/graphql";
import { buildQueueStateClient } from "@/grpc/buildQueueStateClient";
import { env } from "@/utils/env";
import { readableDurationFromDates } from "@/utils/time";
import styles from "./index.module.css";

// ── Recent invocations ───────────────────────────────────────────────────────

const HOME_RECENT_INVOCATIONS = gql(/* GraphQL */ `
  query HomeRecentInvocations($first: Int, $orderBy: BazelInvocationOrder) {
    findBazelInvocations(first: $first, orderBy: $orderBy) {
      edges {
        node {
          id
          invocationID
          startedAt
          endedAt
          exitCodeName
          connectionMetadata {
            connectionLastOpenAt
            timeSinceLastConnectionMillis
          }
          build {
            buildUUID
          }
          metrics {
            actionSummary {
              runnerCount {
                id
                name
                actionsExecuted
              }
            }
          }
        }
      }
    }
  }
`);

const RecentBuildsPanel: React.FC = () => {
  const { data, loading, error } = useApolloQuery(HOME_RECENT_INVOCATIONS, {
    variables: {
      first: 5,
      orderBy: {
        direction: OrderDirection.Desc,
        field: BazelInvocationOrderField.StartedAt,
      },
    },
    fetchPolicy: "cache-and-network",
    pollInterval: 30_000,
  });

  if (loading && !data) {
    return <Skeleton active paragraph={{ rows: 5 }} />;
  }

  if (error) {
    return (
      <PortalAlert
        showIcon
        type="error"
        message="Could not load recent builds"
        description={error.message}
      />
    );
  }

  const invocations = (data?.findBazelInvocations?.edges ?? [])
    .map((e) => e?.node)
    .filter((n): n is NonNullable<typeof n> => n != null);

  if (!invocations.length) {
    return (
      <Typography.Text type="secondary">No builds recorded yet.</Typography.Text>
    );
  }

  return (
    <List
      size="small"
      dataSource={invocations}
      renderItem={(inv) => {
        const runnerCounts = (inv.metrics?.actionSummary?.runnerCount ??
          []) as RunnerCount[];
        const duration =
          inv.startedAt && inv.endedAt
            ? readableDurationFromDates(
                new Date(inv.startedAt),
                new Date(inv.endedAt),
              )
            : null;
        return (
          <List.Item key={inv.id}>
            <List.Item.Meta
              title={
                <Space size="small" wrap>
                  <InvocationResultTag
                    exitCodeName={inv.exitCodeName ?? undefined}
                    timeSinceLastConnectionMillis={
                      inv.connectionMetadata?.timeSinceLastConnectionMillis ??
                      undefined
                    }
                  />
                  <Link
                    to="/bazel-invocations/$invocationID"
                    params={{ invocationID: inv.invocationID }}
                  >
                    <Typography.Text code style={{ fontSize: 12 }}>
                      {inv.invocationID.slice(0, 8)}…
                    </Typography.Text>
                  </Link>
                  {duration && (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {duration}
                    </Typography.Text>
                  )}
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {inv.startedAt
                      ? dayjs(inv.startedAt).format("MMM D, h:mm A")
                      : "—"}
                  </Typography.Text>
                </Space>
              }
              description={
                runnerCounts.length > 0 ? (
                  <div style={{ maxWidth: 200, marginTop: 4 }}>
                    <ExecutionRatioDonut runnerCounts={runnerCounts} />
                  </div>
                ) : null
              }
            />
          </List.Item>
        );
      }}
    />
  );
};

// ── Worker pool ──────────────────────────────────────────────────────────────

const WorkerPoolPanel: React.FC = () => {
  const { data, isPending, isError, error } = useQuery({
    queryKey: ["listPlatformQueues"],
    queryFn: () => buildQueueStateClient.listPlatformQueues({}),
    refetchInterval: 30_000,
  });

  if (isPending) {
    return <Skeleton active paragraph={{ rows: 2 }} />;
  }

  if (isError) {
    return (
      <PortalAlert
        showIcon
        type="error"
        message="Could not load worker pool"
        description={(error as Error).message}
      />
    );
  }

  let totalWorkers = 0;
  let executingWorkers = 0;
  let idleWorkers = 0;
  const offlineQueues: string[] = [];

  for (const queue of data.platformQueues) {
    for (const sc of queue.sizeClassQueues) {
      totalWorkers += sc.workersCount;
      executingWorkers += sc.rootInvocation?.executingWorkersCount ?? 0;
      idleWorkers +=
        (sc.rootInvocation?.idleWorkersCount ?? 0) +
        (sc.rootInvocation?.idleSynchronizingWorkersCount ?? 0);
      if (sc.timeout) {
        const label =
          queue.name?.instanceNamePrefix
            ? `size-${sc.sizeClass} (${queue.name.instanceNamePrefix})`
            : `size-${sc.sizeClass}`;
        offlineQueues.push(label);
      }
    }
  }

  return (
    <Space direction="vertical" style={{ width: "100%" }}>
      {offlineQueues.length > 0 && (
        <Alert
          type="warning"
          showIcon
          icon={<WarningOutlined />}
          message={`Workers offline: ${offlineQueues.join(", ")}`}
        />
      )}
      <Row gutter={[24, 16]}>
        <Col xs={8}>
          <Statistic title="Total" value={totalWorkers} />
        </Col>
        <Col xs={8}>
          <Statistic
            title="Executing"
            value={executingWorkers}
            valueStyle={executingWorkers > 0 ? { color: "#52C41A" } : undefined}
          />
        </Col>
        <Col xs={8}>
          <Statistic
            title="Idle"
            value={idleWorkers}
            valueStyle={{ color: "#8C8C8C" }}
          />
        </Col>
      </Row>
    </Space>
  );
};

// ── Scheduler queue ──────────────────────────────────────────────────────────

const SchedulerQueuePanel: React.FC = () => {
  const { data, isPending, isError, error } = useQuery({
    queryKey: ["listPlatformQueues-scheduler"],
    queryFn: () => buildQueueStateClient.listPlatformQueues({}),
    refetchInterval: 15_000,
  });

  if (isPending) {
    return <Skeleton active paragraph={{ rows: 2 }} />;
  }

  if (isError) {
    return (
      <PortalAlert
        showIcon
        type="error"
        message="Could not load scheduler state"
        description={(error as Error).message}
      />
    );
  }

  let queuedOps = 0;
  let executingOps = 0;

  for (const queue of data.platformQueues) {
    for (const sc of queue.sizeClassQueues) {
      const inv = sc.rootInvocation;
      queuedOps +=
        (inv?.queuedOperationsCount?.direct ?? 0) +
        (inv?.queuedOperationsCount?.indirect ?? 0);
      executingOps += inv?.executingWorkersCount ?? 0;
    }
  }

  return (
    <Row gutter={[24, 16]}>
      <Col xs={12}>
        <Statistic
          title="Queued"
          value={queuedOps}
          valueStyle={queuedOps > 0 ? { color: "#FA8C16" } : undefined}
        />
      </Col>
      <Col xs={12}>
        <Statistic
          title="Executing"
          value={executingOps}
          valueStyle={executingOps > 0 ? { color: "#52C41A" } : undefined}
          valueRender={(v) => <Link to="/operations">{v}</Link>}
        />
      </Col>
    </Row>
  );
};

// ── Uploader / instructions (feature-flagged) ────────────────────────────────

const BuildInstructions: React.FC = () => {
  const bazelrcLines = `build --bes_backend=${env.grpcBackendUrl}\nbuild --bes_results_url=${window.location.origin}/bazel-invocations/`;
  return (
    <Space direction="vertical" size="large">
      <Typography.Text>
        Add the following lines to your{" "}
        <Typography.Text italic>.bazelrc</Typography.Text> to start sending build
        events to the service:
      </Typography.Text>
      <Space size="middle">
        <Typography.Text copyable={{ text: bazelrcLines }} />
        <pre style={{ textAlign: "left" }}>{bazelrcLines}</pre>
      </Space>
    </Space>
  );
};

const BepFileUploader: React.FC = () => (
  <Space direction="vertical" size="large">
    <Uploader
      label="Upload Build Event Protocol (BEP) files to analyze"
      description={
        <Typography.Text type="secondary">
          Upload one or more{" "}
          <Typography.Text type="secondary" italic>
            *.bep.ndjson
          </Typography.Text>{" "}
          file(s) produced with Bazel&apos;s{" "}
          <Typography.Text code>--build_event_json_file</Typography.Text> flag to
          analyze
        </Typography.Text>
      }
      action={"/api/v1/bep/upload"}
    />
  </Space>
);

// ── Home page ────────────────────────────────────────────────────────────────

export function HomePage() {
  const isBesMode = Boolean(env.featureFlags?.bes);

  if (!isBesMode) {
    if (
      env.featureFlags?.home?.fileUpload ||
      env.featureFlags?.home?.instructions
    ) {
      return (
        <Space direction="vertical" size="large" className={styles.container}>
          {!!env.featureFlags?.home?.fileUpload && <BepFileUploader />}
          {!!env.featureFlags?.home?.instructions && <BuildInstructions />}
        </Space>
      );
    }
    if (env.featureFlags?.browser) {
      return <Navigate to="/browser" />;
    }
    if (env.featureFlags?.scheduler) {
      return <Navigate to="/scheduler" />;
    }
    return <Navigate to="/operations" />;
  }

  return (
    <Space direction="vertical" size="large" style={{ width: "100%" }}>
      {!!env.featureFlags?.home?.fileUpload && (
        <PortalCard icon={<DatabaseOutlined />} titleBits={["Upload BEP"]}>
          <BepFileUploader />
        </PortalCard>
      )}
      {!!env.featureFlags?.home?.instructions && (
        <PortalCard icon={<ClockCircleOutlined />} titleBits={["Setup"]}>
          <BuildInstructions />
        </PortalCard>
      )}

      <Row gutter={[24, 24]}>
        <Col xs={24} md={12}>
          <PortalCard
            icon={<ThunderboltOutlined />}
            titleBits={["Worker Pool"]}
          >
            <WorkerPoolPanel />
          </PortalCard>
        </Col>
        <Col xs={24} md={12}>
          <PortalCard
            icon={<BarChartOutlined />}
            titleBits={["Scheduler Queue"]}
            extraBits={[
              <Link key="ops" to="/operations">
                <Typography.Text style={{ fontSize: 12 }}>
                  View all
                </Typography.Text>
              </Link>,
            ]}
          >
            <SchedulerQueuePanel />
          </PortalCard>
        </Col>
      </Row>

      <PortalCard icon={<DatabaseOutlined />} titleBits={["Cache Health"]}>
        <CacheStatsPanel />
      </PortalCard>

      <PortalCard
        icon={<ClockCircleOutlined />}
        titleBits={["Recent Builds"]}
        extraBits={[
          <Link key="all" to="/bazel-invocations">
            <Typography.Text style={{ fontSize: 12 }}>View all</Typography.Text>
          </Link>,
        ]}
      >
        <RecentBuildsPanel />
      </PortalCard>
    </Space>
  );
}
