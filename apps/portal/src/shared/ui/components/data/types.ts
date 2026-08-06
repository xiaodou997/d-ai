export interface DsTableColumn {
  key: string;
  title: string;
  width?: string | number;
  align?: "left" | "center" | "right";
  mono?: boolean;
  /** Allow long descriptive content to wrap; operational columns stay single-line by default. */
  wrap?: boolean;
}
