import { SearchOutlined } from "@ant-design/icons";
import { Link } from "@tanstack/react-router";
import type { FilterValue } from "antd/es/table/interface";
import { SearchFilterIcon, SearchWidget } from "@/components/SearchWidgets";
import {
  OrderDirection,
  type TargetOrder,
  TargetOrderField,
  type TargetWhereInput,
  type TestListRowFragment,
} from "@/graphql/__generated__/graphql";
import type { TableColumnTypeWithFilter } from "@/types/TableColumnTypeWithFilter";
import { readableDurationFromMilliseconds } from "@/utils/time";
import styles from "../../theme/theme.module.css";

// Slowest tests first, unless the URL asks for something else.
export const defaultSorting: TargetOrder = {
  field: TargetOrderField.MaxTestRunDurationInMs,
  direction: OrderDirection.Desc,
};

export const columns: TableColumnTypeWithFilter<
  TestListRowFragment,
  TargetWhereInput
>[] = [
  {
    key: "target",
    title: "Target",
    render: (_, record) => (
      <Link to="/targets/$targetID/tests" params={{ targetID: record.id }}>
        {record.label}
      </Link>
    ),
    filterDropdown: (filterProps) => (
      <SearchWidget placeholder="Target Pattern..." {...filterProps} />
    ),
    filterIcon: (filtered) => (
      <SearchFilterIcon icon={<SearchOutlined />} filtered={filtered} />
    ),
    applyFilter: (value: FilterValue) => {
      if (value.length === 0) {
        return undefined;
      }
      return [
        {
          labelContainsFold: value[0] as string,
        },
      ];
    },
  },
  {
    key: "maxTestRunDurationInMs",
    title: "Duration",
    render: (_, record) => (
      <span className={styles.numberFormat}>
        {record.maxTestRunDurationInMs > 0
          ? readableDurationFromMilliseconds(record.maxTestRunDurationInMs, {
              smallestUnit: "ms",
            })
          : "-"}
      </span>
    ),
    width: 140,
    sortDirections: ["ascend", "descend", "ascend"],
    defaultSortOrder:
      defaultSorting.direction === OrderDirection.Asc ? "ascend" : "descend",
    // Using a dummy sorter function as sorting is handled server-side
    sorter: (_a, _b) => 0,
  },
  {
    key: "instanceName",
    title: "Instance Name",
    filterDropdown: (filterProps) => (
      <SearchWidget placeholder="Instance Name Pattern..." {...filterProps} />
    ),
    filterIcon: (filtered) => (
      <SearchFilterIcon icon={<SearchOutlined />} filtered={filtered} />
    ),
    applyFilter: (value: FilterValue) => {
      if (value.length === 0) {
        return undefined;
      }
      return [
        {
          hasInstanceNameWith: [
            {
              nameContainsFold: value[0] as string,
            },
          ],
        },
      ];
    },
  },
];
