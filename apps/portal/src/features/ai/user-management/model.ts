export interface UserGroupPolicyRow {
  id: string;
  name: string;
  default_user_multiplier: number;
  user_default_visible: boolean;
  user_bound: boolean;
  user_multiplier_override: number | null;
  availability_state: "default" | "custom" | "unavailable";
}

export interface UserPolicyTarget {
  userId: string;
  username: string;
}

export interface UserUsageFilters {
  dateRange: [number, number];
  modelCode: string;
  requestStatus: string;
  requestSource: string;
}

export function defaultUserUsageFilters(): UserUsageFilters {
  const to = new Date();
  const from = new Date();
  from.setDate(from.getDate() - 29);
  from.setHours(0, 0, 0, 0);
  to.setHours(23, 59, 59, 999);
  return {
    dateRange: [from.getTime(), to.getTime()],
    modelCode: "",
    requestStatus: "",
    requestSource: ""
  };
}
