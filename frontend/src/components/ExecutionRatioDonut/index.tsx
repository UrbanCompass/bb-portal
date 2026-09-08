import { theme } from "antd";
import type React from "react";
import { Cell, Legend, Pie, PieChart, Tooltip } from "recharts";
import type { RunnerCount } from "@/graphql/__generated__/graphql";

interface Props {
  runnerCounts: RunnerCount[];
}

const RUNNER_COLORS: Record<string, string> = {
  remote: "#52C41A",
  "remote cache hit": "#1890FF",
  local: "#FA8C16",
};

const RUNNER_LABELS: Record<string, string> = {
  remote: "Remote",
  "remote cache hit": "Cache Hit",
  local: "Local Fallback",
};

interface ChartEntry {
  name: string;
  label: string;
  value: number;
  percent: number;
  color: string;
}

interface CustomTooltipProps {
  active?: boolean;
  payload?: Array<{ payload: ChartEntry }>;
}

const CustomTooltip: React.FC<CustomTooltipProps> = ({ active, payload }) => {
  if (!active || !payload?.length) return null;
  const entry = payload[0].payload;
  return (
    <div
      style={{
        background: "#fff",
        border: "1px solid #d9d9d9",
        borderRadius: 4,
        padding: "8px 12px",
        boxShadow: "0 2px 8px rgba(0,0,0,0.15)",
      }}
    >
      <div style={{ fontWeight: 600, marginBottom: 4 }}>{entry.label}</div>
      <div
        style={{
          fontSize: 24,
          fontWeight: 700,
          color: entry.color,
          lineHeight: 1.2,
        }}
      >
        {entry.percent.toFixed(1)}%
      </div>
      <div style={{ color: "#8C8C8C", fontSize: 12, marginTop: 2 }}>
        {entry.value.toLocaleString()} actions
      </div>
    </div>
  );
};

export const ExecutionRatioDonut: React.FC<Props> = ({ runnerCounts }) => {
  const { token } = theme.useToken();

  const total =
    runnerCounts.find((r) => r.name === "total")?.actionsExecuted ?? 0;

  const data: ChartEntry[] = runnerCounts
    .filter(
      (r): r is typeof r & { name: string } =>
        !!r.name && r.name !== "total" && (r.actionsExecuted ?? 0) > 0,
    )
    .map((r) => {
      const name = r.name;
      const value = r.actionsExecuted ?? 0;
      const percent = total > 0 ? (value / total) * 100 : 0;
      return {
        name,
        label: RUNNER_LABELS[name] ?? name,
        value,
        percent,
        color: RUNNER_COLORS[name] ?? token.colorTextTertiary,
      };
    })
    .sort((a, b) => b.value - a.value);

  if (data.length === 0) return null;

  return (
    <PieChart width={380} height={160}>
      <Pie
        dataKey="value"
        data={data}
        innerRadius={45}
        outerRadius={70}
        cx={80}
        cy={78}
        isAnimationActive={false}
        strokeWidth={1}
        stroke="white"
      >
        {data.map((entry) => (
          <Cell key={entry.name} fill={entry.color} />
        ))}
      </Pie>
      <Tooltip content={<CustomTooltip />} />
      <Legend
        layout="vertical"
        align="right"
        verticalAlign="middle"
        iconType="circle"
        iconSize={10}
        formatter={(_value, entry) => {
          const e = entry.payload as unknown as ChartEntry;
          return (
            <span style={{ fontSize: 13 }}>
              {e.label}: <strong>{e.percent.toFixed(1)}%</strong>
            </span>
          );
        }}
      />
    </PieChart>
  );
};
