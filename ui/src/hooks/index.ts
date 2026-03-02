/**
 * Hooks exports for OpenExec Chat UI
 * @module hooks
 */

export { useWebSocket } from './useWebSocket'
export type {
  WebSocketConfig,
  WebSocketHandlers,
  UseWebSocketReturn,
} from './useWebSocket'

export { useSession } from './useSession'
export type {
  SessionApiConfig,
  UseSessionReturn,
} from './useSession'

export { useMessages } from './useMessages'
export type {
  MessagesApiConfig,
  UseMessagesReturn,
  FetchMessagesOptions,
} from './useMessages'

export { useToolCalls } from './useToolCalls'
export type { UseToolCallsReturn } from './useToolCalls'

export { useChat } from './useChat'
export type { ChatConfig, UseChatReturn } from './useChat'

export { useRestartApproval } from './useRestartApproval'
export type {
  RestartApprovalApiConfig,
  UseRestartApprovalReturn,
} from './useRestartApproval'

export {
  useProviderAvailability,
  clearProviderAvailabilityCache,
} from './useProviderAvailability'
export type {
  ProviderAvailabilityConfig,
  UseProviderAvailabilityReturn,
  ProviderStatus,
  ModelInfoResponse,
  AvailabilityResponse,
} from './useProviderAvailability'

export { useRollback } from './useRollback'
export type {
  RollbackApiConfig,
  UseRollbackReturn,
} from './useRollback'

export { useBackupHistory } from './useBackupHistory'
export type {
  BackupHistoryApiConfig,
  UseBackupHistoryReturn,
} from './useBackupHistory'

export { useUsageData } from './useUsageData'
export type {
  UseUsageDataConfig,
  UseUsageDataReturn,
} from './useUsageData'

export { useFork } from './useFork'
export type {
  ForkApiConfig,
  ForkInfo,
  ForkListItem,
  UseForkReturn,
} from './useFork'
