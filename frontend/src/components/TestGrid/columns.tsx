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
import dayjs from "@/lib/dayjs";
import type { TableColumnTypeWithFilter } from "@/types/TableColumnTypeWithFilter";
import { readableDurationFromMilliseconds } from "@/utils/time";
import styles from "../../theme/theme.module.css";

// Newest tests first, unless the URL asks for something else. This keeps tests
// that were just added at the top of the list.
// The generated TargetOrder has an optional direction. The URL and the table
// always have one.
export type TestSorting = Required<TargetOrder>;

export const defaultSorting: TestSorting = {
  field: TargetOrderField.Id,
  direction: OrderDirection.Desc,
};

// The table column that each sortable field belongs to.
export const sortColumnKeys: Record<TargetOrderField, string> = {
  [TargetOrderField.Id]: "target",
  [TargetOrderField.LastTestBuildAt]: "lastTestBuildAt",
  [TargetOrderField.MaxTestRunDurationInMs]: "maxTestRunDurationInMs",
};

// The sort order of a column, taken from the sort that is in use.
const sortOrderFor = (field: TargetOrderField, sorting: TestSorting) =>
  sorting.field !== field
    ? null
    : sorting.direction === OrderDirection.Asc
      ? ("ascend" as const)
      : ("descend" as const);

export const getColumns = (
  sorting: TestSorting,
): TableColumnTypeWithFilter<TestListRowFragment, TargetWhereInput>[] => [
  {
    key: "target",
    title: "Target",
    // Sorting by target means sorting by when the target was added.
    sortDirections: ["descend", "ascend", "descend"],
    sortOrder: sortOrderFor(TargetOrderField.Id, sorting),
    // Using a dummy sorter function as sorting is handled server-side
    sorter: (_a, _b) => 0,
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
    sortDirections: ["descend", "ascend", "descend"],
    sortOrder: sortOrderFor(TargetOrderField.MaxTestRunDurationInMs, sorting),
    // Using a dummy sorter function as sorting is handled server-side
    sorter: (_a, _b) => 0,
  },
  {
    key: "lastTestBuildAt",
    title: "Last Run",
    render: (_, record) => {
      const at = dayjs(record.lastTestBuildAt);
      // Targets that have not run yet are stored at the Unix epoch.
      return (
        <span className={styles.numberFormat} title={at.format("LLL")}>
          {at.year() > 1970 ? at.fromNow() : "-"}
        </span>
      );
    },
    width: 140,
    sortDirections: ["descend", "ascend", "descend"],
    sortOrder: sortOrderFor(TargetOrderField.LastTestBuildAt, sorting),
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
