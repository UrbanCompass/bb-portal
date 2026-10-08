import {
  BranchesOutlined,
  InfoCircleOutlined,
  SwapOutlined,
} from "@ant-design/icons";
import { useQuery } from "@apollo/client/react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  Alert,
  Badge,
  Button,
  Col,
  Divider,
  Input,
  Row,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import type React from "react";
import { useMemo, useState } from "react";
import { validate as uuidValidate } from "uuid";
import { apolloClient } from "@/components/ApolloWrapper";
import { ExecutionRatioDonut } from "@/components/ExecutionRatioDonut";
import { InvocationResultTag } from "@/components/InvocationResultTag";
import { PortalCard } from "@/components/PortalCard";
import PortalDuration from "@/components/PortalDuration";
import { getFragmentData, gql } from "@/graphql/__generated__";
import type {
  InvocationCompareDataFragment,
  RunnerCount,
} from "@/graphql/__generated__/graphql";
import { buildJoinedRows } from "@/utils/actionComparison";
import type { JoinedActionRow } from "@/utils/actionComparison";
import { generatePageTitle } from "@/utils/generatePageTitle";
import { readableDurationFromMilliseconds } from "@/utils/time";
import z from "zod";

const GET_INVOCATION_FOR_COMPARE = gql(/* GraphQL */ `
  query GetInvocationForCompare($invocationID: UUID!) {
    getBazelInvocation(invocationID: $invocationID) {
      ...InvocationCompareData
    }
  }
`);

const ACTION_COMPARE_FRAGMENT = gql(/* GraphQL */ `
  fragment ActionCompare on Action {
    id
    label
    type
    primaryOutput
    cacheStatus
    sampled
  }
`);

const GET_ACTIONS_FOR_COMPARE = gql(/* GraphQL */ `
  query GetActionsForCompare($invocationID: UUID!) {
    getBazelInvocation(invocationID: $invocationID) {
      id
      actions {
        ...ActionCompare
      }
    }
  }
`);

const INVOCATION_COMPARE_FRAGMENT = gql(/* GraphQL */ `
  fragment InvocationCompareData on BazelInvocation {
    id
    invocationID
    startedAt
    endedAt
    exitCodeName
    connectionMetadata {
      connectionLastOpenAt
      timeSinceLastConnectionMillis
    }
    metrics {
      id
      timingMetrics {
        id
        wallTimeInMs
        criticalPathTimeInMs
        analysisPhaseTimeInMs
        executionPhaseTimeInMs
      }
      actionSummary {
        id
        actionsExecuted
        runnerCount {
          id
          name
          actionsExecuted
        }
      }
    }
    numTotal: invocationTargets {
      totalCount
    }
    numSuccessful: invocationTargets(where: { success: true }) {
      totalCount
    }
    numSkipped: invocationTargets(where: { abortReason: SKIPPED }) {
      totalCount
    }
  }
`);

const CompareSearchSchema = z.object({
  left: z.string().uuid().optional(),
  right: z.string().uuid().optional(),
});

type InvocationData = InvocationCompareDataFragment;

async function fetchInvocation(id: string): Promise<InvocationData | null> {
  const { data } = await apolloClient.query({
    query: GET_INVOCATION_FOR_COMPARE,
    variables: { invocationID: id },
    fetchPolicy: "network-only",
    errorPolicy: "all",
  });
  if (!data?.getBazelInvocation) return null;
  return getFragmentData(INVOCATION_COMPARE_FRAGMENT, data.getBazelInvocation);
}

export const Route = createFileRoute("/bazel-invocations/compare")({
  validateSearch: (search) => CompareSearchSchema.parse(search),
  loaderDeps: ({ search: { left, right } }) => ({ left, right }),
  loader: async ({ deps: { left, right } }) => {
    const [leftInv, rightInv] = await Promise.all([
      left ? fetchInvocation(left) : Promise.resolve(null),
      right ? fetchInvocation(right) : Promise.resolve(null),
    ]);
    return { leftInv, rightInv };
  },
  head: () => ({
    meta: [{ title: generatePageTitle(["Invocation Comparison"]) }],
  }),
  component: RouteComponent,
});

function RouteComponent() {
  const { left, right } = Route.useSearch();
  const { leftInv, rightInv } = Route.useLoaderData();
  const navigate = useNavigate({ from: Route.fullPath });

  const [rightInput, setRightInput] = useState(right ?? "");
  const [rightInputError, setRightInputError] = useState<string | null>(null);

  const handleRightSearch = () => {
    const val = rightInput.trim();
    if (!val) {
      navigate({ search: (prev) => ({ ...prev, right: undefined }) });
      return;
    }
    if (!uuidValidate(val)) {
      setRightInputError("Must be a valid UUID");
      return;
    }
    setRightInputError(null);
    navigate({ search: (prev) => ({ ...prev, right: val }) });
  };

  const leftTotal =
    leftInv?.metrics?.actionSummary?.actionsExecuted ?? undefined;
  const rightTotal =
    rightInv?.metrics?.actionSummary?.actionsExecuted ?? undefined;

  return (
    <PortalCard icon={<SwapOutlined />} titleBits={["Invocation Comparison"]}>
      <Row gutter={[24, 24]}>
        <Col xs={24} md={12}>
          <InvocationColumn
            label="Left"
            invocationID={left}
            invocation={leftInv}
          />
        </Col>
        <Col xs={24} md={12}>
          <Space direction="vertical" style={{ width: "100%" }}>
            {right && rightInv === null && (
              <Alert
                type="error"
                message={`Invocation "${right}" not found`}
                showIcon
              />
            )}
            {right && rightInv ? (
              <InvocationColumn
                label="Right"
                invocationID={right}
                invocation={rightInv}
                compareWith={leftInv ?? undefined}
              />
            ) : (
              <PortalCard
                type="inner"
                icon={<InfoCircleOutlined />}
                titleBits={["Select right invocation"]}
              >
                <Space.Compact style={{ width: "100%" }}>
                  <Input
                    placeholder="Paste an invocation UUID..."
                    value={rightInput}
                    onChange={(e) => {
                      setRightInput(e.target.value);
                      setRightInputError(null);
                    }}
                    onPressEnter={handleRightSearch}
                    status={rightInputError ? "error" : undefined}
                  />
                  <Button type="primary" onClick={handleRightSearch}>
                    Compare
                  </Button>
                </Space.Compact>
                {rightInputError && (
                  <Typography.Text type="danger" style={{ fontSize: 12 }}>
                    {rightInputError}
                  </Typography.Text>
                )}
              </PortalCard>
            )}
          </Space>
        </Col>
      </Row>
      {left && right && leftInv && rightInv && (
        <>
          <Divider />
          <ActionComparisonPanel
            leftID={left}
            rightID={right}
            leftTotal={leftTotal}
            rightTotal={rightTotal}
          />
        </>
      )}
    </PortalCard>
  );
}

function totalActions(runnerCounts: RunnerCount[]): number {
  return runnerCounts.reduce((s, r) => s + (r.actionsExecuted ?? 0), 0);
}

function runnerPercent(runnerCounts: RunnerCount[], name: string): number {
  const total = totalActions(runnerCounts);
  if (total === 0) return 0;
  const match = runnerCounts.find(
    (r) => r.name?.toLowerCase() === name.toLowerCase(),
  );
  return ((match?.actionsExecuted ?? 0) / total) * 100;
}

const DELTA_THRESHOLD_RATIO = 0.1; // 10 percentage-point difference triggers highlight

function deltaStyle(
  leftPct: number,
  rightPct: number,
): React.CSSProperties | undefined {
  if (Math.abs(leftPct - rightPct) >= DELTA_THRESHOLD_RATIO * 100) {
    return { background: "#fff7e6", borderRadius: 4, padding: "2px 6px" };
  }
  return undefined;
}

interface InvocationColumnProps {
  label: string;
  invocationID: string | undefined;
  invocation: InvocationData | null;
  compareWith?: InvocationData;
}

const InvocationColumn: React.FC<InvocationColumnProps> = ({
  label,
  invocationID,
  invocation,
  compareWith,
}) => {
  if (!invocationID) {
    return (
      <PortalCard type="inner" icon={<InfoCircleOutlined />} titleBits={[label]}>
        <Typography.Text type="secondary">No invocation selected</Typography.Text>
      </PortalCard>
    );
  }
  if (!invocation) {
    return (
      <PortalCard type="inner" icon={<InfoCircleOutlined />} titleBits={[label]}>
        <Alert
          type="error"
          message={`Invocation "${invocationID}" not found`}
          showIcon
        />
      </PortalCard>
    );
  }

  const runnerCounts = (invocation.metrics?.actionSummary?.runnerCount ??
    []) as RunnerCount[];
  const cacheHitPct = runnerPercent(runnerCounts, "remote cache hit");
  const remotePct = runnerPercent(runnerCounts, "remote");
  const localPct = runnerPercent(runnerCounts, "local");

  const compareCacheHitPct = compareWith
    ? runnerPercent(
        (compareWith.metrics?.actionSummary?.runnerCount ?? []) as RunnerCount[],
        "remote cache hit",
      )
    : undefined;
  const compareLocalPct = compareWith
    ? runnerPercent(
        (compareWith.metrics?.actionSummary?.runnerCount ?? []) as RunnerCount[],
        "local",
      )
    : undefined;

  const timing = invocation.metrics?.timingMetrics;

  return (
    <PortalCard
      type="inner"
      icon={<SwapOutlined />}
      titleBits={[
        <span key="label">
          {label}:{" "}
          <Link
            to="/bazel-invocations/$invocationID"
            params={{ invocationID: invocation.invocationID }}
          >
            <Typography.Text code style={{ fontSize: 12, verticalAlign: "middle" }}>
              {invocation.invocationID}
            </Typography.Text>
          </Link>
        </span>,
      ]}
    >
      <Space direction="vertical" size="large" style={{ width: "100%" }}>
        {/* Status + Duration */}
        <Space size="middle" wrap>
          <InvocationResultTag
            exitCodeName={invocation.exitCodeName ?? undefined}
            timeSinceLastConnectionMillis={
              invocation.connectionMetadata?.timeSinceLastConnectionMillis ??
              undefined
            }
          />
          <PortalDuration
            from={invocation.startedAt ?? undefined}
            to={
              invocation.endedAt ??
              invocation.connectionMetadata?.connectionLastOpenAt ??
              undefined
            }
            includeIcon
            formatConfig={{ smallestUnit: "s" }}
          />
        </Space>

        {/* Execution ratio */}
        {runnerCounts.length > 0 && (
          <>
            <Divider orientation="left" orientationMargin={0}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Execution Ratio
              </Typography.Text>
            </Divider>
            <ExecutionRatioDonut runnerCounts={runnerCounts} />
            <Space size="middle" wrap>
              <Tag
                color="blue"
                style={
                  compareCacheHitPct !== undefined
                    ? deltaStyle(cacheHitPct, compareCacheHitPct)
                    : undefined
                }
              >
                Cache Hit {cacheHitPct.toFixed(1)}%
              </Tag>
              <Tag color="green">Remote {remotePct.toFixed(1)}%</Tag>
              <Tag
                color="orange"
                style={
                  compareLocalPct !== undefined
                    ? deltaStyle(localPct, compareLocalPct)
                    : undefined
                }
              >
                Local {localPct.toFixed(1)}%
              </Tag>
            </Space>
          </>
        )}

        {/* Target counts */}
        <Divider orientation="left" orientationMargin={0}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            Targets
          </Typography.Text>
        </Divider>
        <Space size="large" wrap>
          {invocation.numTotal?.totalCount !== undefined && (
            <Statistic
              title="Analyzed"
              value={invocation.numTotal.totalCount}
            />
          )}
          {invocation.numSuccessful?.totalCount !== undefined && (
            <Statistic
              title="Successful"
              value={invocation.numSuccessful.totalCount}
              valueStyle={{ color: "green" }}
            />
          )}
          {invocation.numSkipped?.totalCount !== undefined && (
            <Statistic
              title="Skipped"
              value={invocation.numSkipped.totalCount}
              valueStyle={{ color: "purple" }}
            />
          )}
        </Space>

        {/* Timing metrics */}
        {timing && (
          <>
            <Divider orientation="left" orientationMargin={0}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Timing
              </Typography.Text>
            </Divider>
            <Space size="large" wrap>
              <Statistic
                title="Wall Time"
                value={readableDurationFromMilliseconds(
                  timing.wallTimeInMs ?? 0,
                  { smallestUnit: "ms" },
                )}
              />
              <Statistic
                title="Critical Path"
                value={readableDurationFromMilliseconds(
                  timing.criticalPathTimeInMs ?? 0,
                  { smallestUnit: "ms" },
                )}
              />
              <Statistic
                title="Analysis"
                value={readableDurationFromMilliseconds(
                  timing.analysisPhaseTimeInMs ?? 0,
                  { smallestUnit: "ms" },
                )}
              />
              <Statistic
                title="Execution"
                value={readableDurationFromMilliseconds(
                  timing.executionPhaseTimeInMs ?? 0,
                  { smallestUnit: "ms" },
                )}
              />
            </Space>
          </>
        )}
      </Space>
    </PortalCard>
  );
};

// ─── Action Comparison Panel ────────────────────────────────────────────────


function CacheStatusCell({
  status,
  present,
}: {
  status: string | null | undefined;
  present: boolean;
}) {
  if (!present) {
    return <Badge status="default" text="not sampled" />;
  }
  if (!status) {
    return <Typography.Text type="secondary">—</Typography.Text>;
  }
  const color =
    status === "remote cache hit"
      ? "blue"
      : status === "remote"
        ? "green"
        : "orange";
  return <Tag color={color}>{status}</Tag>;
}

interface ActionComparisonPanelProps {
  leftID: string;
  rightID: string;
  leftTotal: number | undefined;
  rightTotal: number | undefined;
}

const ActionComparisonPanel: React.FC<ActionComparisonPanelProps> = ({
  leftID,
  rightID,
  leftTotal,
  rightTotal,
}) => {
  const { data: leftData, loading: leftLoading } = useQuery(
    GET_ACTIONS_FOR_COMPARE,
    { variables: { invocationID: leftID }, fetchPolicy: "cache-first" },
  );
  const { data: rightData, loading: rightLoading } = useQuery(
    GET_ACTIONS_FOR_COMPARE,
    { variables: { invocationID: rightID }, fetchPolicy: "cache-first" },
  );

  const leftActions = useMemo(
    () =>
      (leftData?.getBazelInvocation?.actions ?? []).map((a) =>
        getFragmentData(ACTION_COMPARE_FRAGMENT, a),
      ),
    [leftData],
  );
  const rightActions = useMemo(
    () =>
      (rightData?.getBazelInvocation?.actions ?? []).map((a) =>
        getFragmentData(ACTION_COMPARE_FRAGMENT, a),
      ),
    [rightData],
  );

  const rows = useMemo(
    () => buildJoinedRows(leftActions, rightActions),
    [leftActions, rightActions],
  );

  const matchedCount = rows.filter((r) => r.onLeft && r.onRight).length;

  const columns: ColumnsType<JoinedActionRow> = [
    {
      title: "Label",
      dataIndex: "label",
      ellipsis: true,
      render: (label: string) => (
        <Typography.Text code style={{ fontSize: 11 }}>
          {label}
        </Typography.Text>
      ),
    },
    {
      title: "Mnemonic",
      dataIndex: "mnemonic",
      width: 120,
      render: (v: string | null) =>
        v ? <Tag>{v}</Tag> : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: "Primary output",
      dataIndex: "primaryOutput",
      ellipsis: true,
      render: (v: string | null) =>
        v ? (
          <Typography.Text code style={{ fontSize: 11 }}>
            {v}
          </Typography.Text>
        ) : (
          <Typography.Text type="secondary">—</Typography.Text>
        ),
    },
    {
      title: "Left cache status",
      width: 160,
      render: (_: unknown, row: JoinedActionRow) => (
        <CacheStatusCell status={row.leftStatus} present={row.onLeft} />
      ),
    },
    {
      title: "Right cache status",
      width: 160,
      render: (_: unknown, row: JoinedActionRow) => (
        <CacheStatusCell status={row.rightStatus} present={row.onRight} />
      ),
    },
  ];

  const coverageText = (() => {
    const parts: string[] = [`${matchedCount} action${matchedCount !== 1 ? "s" : ""} matched`];
    if (leftTotal !== undefined || rightTotal !== undefined) {
      const denom = [
        leftTotal !== undefined ? `~${leftTotal} left` : null,
        rightTotal !== undefined ? `~${rightTotal} right` : null,
      ]
        .filter(Boolean)
        .join(" / ");
      parts.push(`of ${denom} total`);
    }
    parts.push("(sample-based — not exhaustive)");
    return parts.join(" · ");
  })();

  if (leftLoading || rightLoading) {
    return (
      <Space direction="vertical" style={{ width: "100%" }} align="center">
        <Spin />
        <Typography.Text type="secondary">Loading actions…</Typography.Text>
      </Space>
    );
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: "100%" }}>
      <Space>
        <BranchesOutlined />
        <Typography.Text strong>Action Comparison</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {coverageText}
        </Typography.Text>
      </Space>

      {rows.length === 0 ? (
        <Alert
          type="info"
          message="No sampled actions found"
          description="Neither invocation had actions persisted in the sample. Failed actions are always recorded; successful actions are a random sample (up to 100 per invocation)."
          showIcon
        />
      ) : (
        <Table<JoinedActionRow>
          dataSource={rows}
          columns={columns}
          size="small"
          pagination={{ pageSize: 50, hideOnSinglePage: true }}
          rowClassName={(row) => {
            if (!row.onLeft || !row.onRight) return "";
            if (row.leftStatus !== row.rightStatus)
              return "action-compare-row-diff";
            return "action-compare-row-same";
          }}
          style={{ overflow: "auto" }}
        />
      )}

      <style>{`
        .action-compare-row-diff td {
          background: #fff7e6 !important;
        }
        .action-compare-row-same td {
          opacity: 0.6;
        }
        [data-theme="dark"] .action-compare-row-diff td {
          background: #2b2000 !important;
        }
      `}</style>
    </Space>
  );
};
