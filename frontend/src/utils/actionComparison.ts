export interface ActionForJoin {
  id: string;
  label: string;
  type?: string | null;
  primaryOutput?: string | null;
  cacheStatus?: string | null;
}

export interface JoinedActionRow {
  key: string;
  label: string;
  mnemonic: string | null | undefined;
  primaryOutput: string | null | undefined;
  leftStatus: string | null | undefined;
  rightStatus: string | null | undefined;
  onLeft: boolean;
  onRight: boolean;
}

export function joinKey(a: ActionForJoin): string {
  return `${a.label}||${a.type ?? ""}||${a.primaryOutput ?? ""}`;
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
      leftStatus: l?.cacheStatus ?? null,
      rightStatus: r?.cacheStatus ?? null,
      onLeft: !!l,
      onRight: !!r,
    });
  }

  // Differing-status matched rows first, then same-status matched, then one-sided
  rows.sort((a, b) => {
    const aDiff = a.onLeft && a.onRight && a.leftStatus !== a.rightStatus;
    const bDiff = b.onLeft && b.onRight && b.leftStatus !== b.rightStatus;
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
