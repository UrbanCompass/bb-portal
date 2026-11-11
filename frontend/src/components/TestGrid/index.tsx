import type { SorterResult } from "antd/es/table/interface";
import type React from "react";
import type {
  TargetWhereInput,
  TestListRowFragment,
} from "@/graphql/__generated__/graphql";
import { PageCursorTable } from "../PageCursorTable";
import type {
  GetPaginationUpdateLinkType,
  PageInfo,
} from "../PageCursorTable/types";
import { tableFiltersToGraphqlWhere } from "../PageCursorTable/utils";
import TestGridRow from "../TestGridRow";
import { columns } from "./columns";

interface Props {
  pageSize: number;
  onFilterChange: (where: TargetWhereInput[]) => void;
  onSortChange: (
    sorter:
      | SorterResult<TestListRowFragment>
      | SorterResult<TestListRowFragment>[],
  ) => void;
  getPaginationUpdateLink: GetPaginationUpdateLinkType;
  pageInfo: PageInfo;
  testsData: TestListRowFragment[];
}

const TestGrid: React.FC<Props> = ({
  pageSize,
  onFilterChange,
  onSortChange,
  getPaginationUpdateLink,
  pageInfo,
  testsData,
}) => {
  return (
    <PageCursorTable<TestListRowFragment>
      columns={columns}
      dataSource={testsData}
      size="small"
      rowKey="id"
      expandable={{
        expandedRowRender: (record) => (
          <TestGridRow
            instanceName={record.instanceName.name}
            label={record.label}
            aspect={record.aspect}
            targetKind={record.targetKind}
            numberOfElements={40}
            direction={"newToOld"}
          />
        ),
        rowExpandable: (_) => true,
      }}
      showSorterTooltip={{ target: "sorter-icon" }}
      onChange={(_pagination, filters, sorter, _extra) => {
        onSortChange(sorter);
        onFilterChange(tableFiltersToGraphqlWhere(columns, filters));
      }}
      pageInfo={pageInfo}
      getPaginationUpdateLink={getPaginationUpdateLink}
      pageSize={pageSize}
    />
  );
};

export default TestGrid;
