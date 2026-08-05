export interface DsTableColumn {
  key: string;
  title: string;
  width?: string | number;
  align?: "left" | "center" | "right";
  mono?: boolean;
}
