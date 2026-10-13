export interface ActionForJoin {
  id: string;
  label: string;
  type?: string | null;
  primaryOutput?: string | null;
  success?: boolean | null;
  exitCode?: number | null;
  startTime?: string | null;
  endTime?: string | null;
}

export interface JoinedActionRow {
  key: string;
  label: string;
  mnemonic: string | null | undefined;
  primaryOutput: string | null | undefined;
  leftOutcome: string | null;
  rightOutcome: string | null;
  leftDurationMs: number | null;
  rightDurationMs: number | null;
  onLeft: boolean;
  onRight: boolean;
}

export function joinKey(a: ActionForJoin): string {
  return `${a.label}||${a.type ?? ""}||${a.primaryOutput ?? ""}`;
}

// actionOutcome summarizes how an action finished: "success", or the exit
// code for failed actions.
export function actionOutcome(a: ActionForJoin | undefined): string | null {
  if (!a) return null;
  if (a.success) return "success";
  return `exit ${a.exitCode ?? "?"}`;
}

export function actionDurationMs(a: ActionForJoin | undefined): number | null {
  if (!a?.startTime || !a.endTime) return null;
  const ms = Date.parse(a.endTime) - Date.parse(a.startTime);
  return Number.isNaN(ms) || ms < 0 ? null : ms;
}

export function buildJoinedRows(
  leftActions: ActionForJoin[],
  rightActions: ActionForJoin[],
): JoinedActionRow[] {
  const leftMap = new Map<string, ActionForJoin>();
  for (const a of leftActions) leftMap.set(joinKey(a), a);

  const rightMap = new Map<string, ActionForJoin>();
  for (const a of rightActions) rightMap.set(joinKey(a), a);

  const allKeys = new Set([...leftMap.keys(), ...rightMap.keys()]);
  const rows: JoinedActionRow[] = [];

  for (const k of allKeys) {
    const l = leftMap.get(k);
    const r = rightMap.get(k);
    const ref = l ?? r!;
    rows.push({
      key: k,
      label: ref.label,
      mnemonic: ref.type,
      primaryOutput: ref.primaryOutput,
      leftOutcome: actionOutcome(l),
      rightOutcome: actionOutcome(r),
      leftDurationMs: actionDurationMs(l),
      rightDurationMs: actionDurationMs(r),
      onLeft: !!l,
      onRight: !!r,
    });
  }

  // Differing-outcome matched rows first, then same-outcome matched, then one-sided
  rows.sort((a, b) => {
    const aDiff = a.onLeft && a.onRight && a.leftOutcome !== a.rightOutcome;
    const bDiff = b.onLeft && b.onRight && b.leftOutcome !== b.rightOutcome;
    if (aDiff && !bDiff) return -1;
    if (!aDiff && bDiff) return 1;
    const aBoth = a.onLeft && a.onRight;
    const bBoth = b.onLeft && b.onRight;
    if (aBoth && !bBoth) return -1;
    if (!aBoth && bBoth) return 1;
    return a.label.localeCompare(b.label);
  });

  return rows;
}
