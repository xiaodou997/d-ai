import {
  ArrowDown,
  ArrowUp,
  ChatDotRound,
  CopyDocument,
  Delete,
  Download,
  Edit,
  EditPen,
  Opportunity,
  Plus,
  Promotion,
  Refresh,
  RefreshRight,
  Setting,
  Upload
} from "@element-plus/icons-vue";

export const portalApiKeyWorkspaceIconProps = {
  refreshIcon: Refresh,
  createIcon: Plus,
  editIcon: Edit,
  deleteIcon: Delete,
  copyIcon: CopyDocument
};

export const portalAppKeyWorkspaceIconProps = {
  refreshIcon: Refresh,
  createIcon: Plus,
  copyIcon: CopyDocument,
  editIcon: EditPen,
  deleteIcon: Delete
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
  agentNoteIcon: Opportunity,
  emptyIcon: ChatDotRound
};

export const portalAppManagementIconProps = {
  refreshIcon: RefreshRight,
  createIcon: Plus,
  editIcon: EditPen,
  deleteIcon: Delete,
  importIcon: Upload,
  exportIcon: Download
};

export const portalImageWorkspaceIconProps = {
  refreshIcon: Refresh,
  submitIcon: Promotion
};

export const portalVisibleGroupsWorkspaceIconProps = {
  refreshIcon: Refresh
};
