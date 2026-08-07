import {
  ArrowDown,
  ArrowUp,
  ChatDotRound,
  CopyDocument,
  Delete,
  Edit,
  Plus,
  Promotion,
  Refresh,
  Setting
} from "@element-plus/icons-vue";

export const portalApiKeyWorkspaceIconProps = {
  refreshIcon: Refresh,
  createIcon: Plus,
  editIcon: Edit,
  deleteIcon: Delete,
  copyIcon: CopyDocument
};

export const portalChatWorkspaceIconProps = {
  refreshIcon: Refresh,
  createIcon: Plus,
  copyIcon: CopyDocument,
  sendIcon: Promotion,
  clearIcon: Delete,
  deleteIcon: Delete,
  collapseOpenIcon: ArrowUp,
  collapseClosedIcon: ArrowDown,
  settingsIcon: Setting,
  modelNoteIcon: ChatDotRound,
  emptyIcon: ChatDotRound
};

export const portalImageWorkspaceIconProps = {
  refreshIcon: Refresh,
  submitIcon: Promotion
};

export const portalVisibleGroupsWorkspaceIconProps = {
  refreshIcon: Refresh
};
