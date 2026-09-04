export type WorkbenchRangeId = "24h" | "today" | "yesterday" | "thisMonth" | "lastMonth" | "7d" | "30d" | "90d" | "custom";

export interface WorkbenchRangeOption {
  id: WorkbenchRangeId;
  label: string;
  caption: string;
  hours: number;
  sampleLimit: number;
}

export interface WorkbenchRangeWindow {
  startAt: Date;
  endAt: Date;
  startTime: number;
  endTime: number;
  date_from: string;
  date_to: string;
}

export const DEFAULT_WORKBENCH_RANGE_ID: WorkbenchRangeId = "30d";

export const WORKBENCH_RANGE_OPTIONS: WorkbenchRangeOption[] = [
  { id: "today", label: "今天", caption: "从今天 00:00 起的自然日用量", hours: 24, sampleLimit: 120 },
  { id: "yesterday", label: "昨天", caption: "完整查看昨天自然日用量", hours: 24, sampleLimit: 120 },
  { id: "thisMonth", label: "本月", caption: "从本月 1 日起的自然月用量", hours: 720, sampleLimit: 360 },
  { id: "lastMonth", label: "上月", caption: "完整查看上一个自然月用量", hours: 720, sampleLimit: 360 },
  { id: "7d", label: "最近七天", caption: "观察周内调用节奏", hours: 168, sampleLimit: 240 },
  { id: "30d", label: "近30天", caption: "判断月度结构与主力模型", hours: 720, sampleLimit: 360 },
  { id: "90d", label: "近90天", caption: "拉长看季度趋势与沉淀", hours: 2160, sampleLimit: 480 },
  { id: "custom", label: "自定义", caption: "选择任意起止时间", hours: 720, sampleLimit: 480 }
];

export const isWorkbenchRangeId = (value: string): value is WorkbenchRangeId =>
  WORKBENCH_RANGE_OPTIONS.some((option) => option.id === value);

export const getWorkbenchRangeOption = (id: string): WorkbenchRangeOption =>
  WORKBENCH_RANGE_OPTIONS.find((option) => option.id === id) ??
  WORKBENCH_RANGE_OPTIONS.find((option) => option.id === DEFAULT_WORKBENCH_RANGE_ID) ??
  WORKBENCH_RANGE_OPTIONS[0];

export const buildWorkbenchRangeWindow = (range: WorkbenchRangeOption): WorkbenchRangeWindow => {
  const now = new Date();
  const startAt = new Date(now);
  const endAt = new Date(now);
  if (range.id === "today") {
    startAt.setHours(0, 0, 0, 0);
  } else if (range.id === "yesterday") {
    startAt.setDate(startAt.getDate() - 1);
    startAt.setHours(0, 0, 0, 0);
    endAt.setHours(0, 0, 0, 0);
  } else if (range.id === "thisMonth") {
    startAt.setDate(1); startAt.setHours(0, 0, 0, 0);
  } else if (range.id === "lastMonth") {
    startAt.setMonth(startAt.getMonth() - 1, 1); startAt.setHours(0, 0, 0, 0);
    endAt.setDate(1); endAt.setHours(0, 0, 0, 0);
  } else {
    startAt.setTime(endAt.getTime() - range.hours * 60 * 60 * 1000);
  }

  return {
    startAt,
    endAt,
    startTime: startAt.getTime(),
    endTime: endAt.getTime(),
    date_from: startAt.toISOString(),
    date_to: endAt.toISOString()
  };
};
