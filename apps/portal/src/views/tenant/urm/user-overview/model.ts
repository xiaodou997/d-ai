export interface UserOverviewAccessibleGroup {
  id: string;
  name: string;
  source: "default" | "custom";
  effectiveUserMultiplier: number;
  defaultMultiplier: number | null;
  overrideMultiplier: number | null;
  defaultVisible: boolean;
}

export interface UserOverviewGroupSummary {
  totalAvailable: number;
  accessible: number;
  defaultVisible: number;
  customBindings: number;
}

export interface UserOverviewRiskSignal {
  id: string;
  tone: "success" | "warning" | "danger" | "info";
  title: string;
  value: string;
  description: string;
}
