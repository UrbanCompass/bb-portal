import { createFileRoute, linkOptions } from "@tanstack/react-router";
import type { SorterResult } from "antd/es/table/interface";
import z from "zod";
import { apolloClient } from "@/components/ApolloWrapper";
import { DEFAULT_PAGE_SIZE } from "@/components/PageCursorTable";
import {
  type TablePaginationVars,
  TablePaginationVarsSchema,
} from "@/components/PageCursorTable/types";
import { TestsPage } from "@/components/pages/Tests";
import {
  defaultSorting,
  sortColumnKeys,
  type TestSorting,
} from "@/components/TestGrid/columns";
import { gql } from "@/graphql/__generated__/gql";
import {
  OrderDirection,
  TargetOrderField,
  type TargetWhereInput,
  type TestListRowFragment,
} from "@/graphql/__generated__/graphql";
import {
  TargetOrderSchema,
  TargetWhereInputSchema,
} from "@/graphql/__generated__/zod";
import { NotFoundError } from "@/main";
import { generatePageTitle } from "@/utils/generatePageTitle";
import { parseGraphqlEdgeListWithFragment } from "@/utils/parseGraphqlEdgeList";

export const GET_TESTS = gql(/* GraphQl */ `
  query GetTests(
    $after: Cursor
    $first: Int
    $before: Cursor
    $last: Int
    $where: TargetWhereInput
    $orderBy: TargetOrder
  ) {
    findTargets(
      after: $after
      first: $first
      before: $before
      last: $last
      where: $where
      orderBy: $orderBy
    ) {
      pageInfo {
        startCursor
        endCursor
        hasNextPage
        hasPreviousPage
      }
      edges {
        node {
          ...TestListRow
        }
      }
    }
  }
`);

const TESTS_FRAGMENT = gql(/* GraphQL */ `
  fragment TestListRow on Target {
    id
    label
    aspect
    targetKind
    maxTestRunDurationInMs
    lastTestBuildAt
    instanceName {
      name
    }
  }
`);

const TestSearchSchema = z.object({
  testTable: TablePaginationVarsSchema.extend({
    where: z.array(TargetWhereInputSchema().partial()).optional(),
    orderBy: TargetOrderSchema().optional(),
  }).optional(),
});

export const Route = createFileRoute("/tests/")({
  component: RouteComponent,
  validateSearch: (search) => TestSearchSchema.parse(search),
  loaderDeps: ({ search: { testTable } }) => ({ testTable }),
  loader: async ({ deps }) => {
    // We set the defaults here instead of in the validate search function, as
    // that updates the URL and we don't want to do that on initial load.
    const pageSize = deps.testTable?.pageSize ?? DEFAULT_PAGE_SIZE;
    const pagination = deps.testTable?.pagination ?? { first: pageSize };
    const where = deps.testTable?.where ?? [];
    const orderBy: TestSorting = {
      ...defaultSorting,
      ...deps.testTable?.orderBy,
    };

    const { data, error } = await apolloClient.query({
      errorPolicy: "all",
      query: GET_TESTS,
      variables: {
        where: {
          and: [{ hasTestTarget: true }, ...where],
        },
        orderBy,
        ...pagination,
      },
    });
    if (!data) {
      // TODO: Should this perhaps give a "failed to connect to server" error instead?
      throw new NotFoundError("tests", error?.message);
    }
    const testsData: TestListRowFragment[] = parseGraphqlEdgeListWithFragment(
      TESTS_FRAGMENT,
      data?.findTargets,
    );

    return {
      testsData,
      pageSize,
      orderBy,
      pageInfo: data.findTargets.pageInfo,
    };
  },
  head: (_ctx) => ({ meta: [{ title: generatePageTitle(["Tests"]) }] }),
});

const getPaginationUpdateLink = (newPagination: TablePaginationVars) =>
  linkOptions({
    from: Route.id,
    to: ".",
    search: (prev): typeof prev => ({
      ...prev,
      testTable: {
        ...prev.testTable,
        ...newPagination,
      },
    }),
  });

function RouteComponent() {
  const navigate = Route.useNavigate();
  const { testsData, pageSize, orderBy, pageInfo } = Route.useLoaderData();
  const onFilterChange = (where: TargetWhereInput[]) => {
    navigate({
      search: (prev): typeof prev => ({
        ...prev,
        testTable: {
          ...prev.testTable,
          where,
          pagination: undefined,
        },
      }),
    });
  };
  const onSortChange = (
    sorter:
      | SorterResult<TestListRowFragment>
      | SorterResult<TestListRowFragment>[],
  ) => {
    const s = Array.isArray(sorter) ? sorter[0] : sorter;
    const field = Object.values(TargetOrderField).find(
      (f) => sortColumnKeys[f] === s?.columnKey,
    );
    // Cleared sort or a column that can not be sorted: use the default order.
    const orderBy: TestSorting =
      !s?.order || !field
        ? defaultSorting
        : {
            field,
            direction:
              s.order === "ascend" ? OrderDirection.Asc : OrderDirection.Desc,
          };
    navigate({
      search: (prev): typeof prev => ({
        ...prev,
        testTable: {
          ...prev.testTable,
          orderBy,
          pagination: undefined,
        },
      }),
    });
  };
  return (
    <TestsPage
      testsData={testsData}
      pageSize={pageSize}
      sorting={orderBy}
      pageInfo={pageInfo}
      getPaginationUpdateLink={getPaginationUpdateLink}
      onFilterChange={onFilterChange}
      onSortChange={onSortChange}
    />
  );
}
