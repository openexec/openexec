/**
 * ForkContext - Context provider for session fork operations
 *
 * Provides fork capabilities throughout the component tree:
 * - Fork dialog state management
 * - Fork operation execution
 * - Fork info retrieval
 * - Navigation to forked sessions
 *
 * @module components/chat/session/ForkContext
 */

import React, { createContext, useContext, useCallback, useMemo } from 'react'
import type { Session, Message, ProviderInfo } from '../../../types/chat'
import type { ForkOptions, ForkResult } from './SessionForkDialog'
import type { ForkInfo, ForkListItem } from '../../../hooks/useFork'
import { useFork } from '../../../hooks/useFork'
import SessionForkDialog from './SessionForkDialog'

// =============================================================================
// Context Types
// =============================================================================

export interface ForkContextValue {
  /** Whether the fork dialog is currently open */
  isForkDialogOpen: boolean
  /** Session currently being forked (if any) */
  sessionToFork: Session | undefined
  /** Pre-selected fork point message (if any) */
  forkPointMessage: Message | undefined
  /** Whether a fork operation is in progress */
  isForking: boolean
  /** Error from the last fork operation (if any) */
  forkError: string | undefined
  /** Result of the last successful fork */
  lastForkResult: ForkResult | undefined
  /** Fork info for the current session */
  forkInfo: ForkInfo | undefined
  /** Whether fork info is loading */
  forkInfoLoading: boolean
  /** List of forks for the current session */
  sessionForks: ForkListItem[]
  /** Whether session forks are loading */
  sessionForksLoading: boolean
  /** Open the fork dialog for a session */
  openForkDialog: (session: Session, forkPointMessage?: Message) => void
  /** Close the fork dialog */
  closeForkDialog: () => void
  /** Get fork info for a session */
  getForkInfo: (sessionId: string) => Promise<ForkInfo>
  /** List all forks of a session */
  listSessionForks: (sessionId: string) => Promise<ForkListItem[]>
  /** Clear any fork error */
  clearForkError: () => void
}

// =============================================================================
// Context Definition
// =============================================================================

const ForkContext = createContext<ForkContextValue | undefined>(undefined)

// =============================================================================
// Provider Props
// =============================================================================

export interface ForkProviderProps {
  /** Child components */
  children: React.ReactNode
  /** Base URL for fork API endpoints */
  apiBaseUrl: string
  /** Optional auth token for API requests */
  authToken?: string
  /** Available providers for fork dialog provider selection */
  providers?: ProviderInfo[]
  /** Messages for fork point selection (if not using pre-selected) */
  messages?: Message[]
  /** Callback when fork is completed successfully */
  onForkComplete?: (result: ForkResult) => void
  /** Callback to navigate to a forked session */
  onNavigateToSession?: (sessionId: string) => void
}

// =============================================================================
// Provider Component
// =============================================================================

/**
 * ForkProvider - Provides fork functionality to child components
 *
 * Wraps the useFork hook and SessionForkDialog to provide a complete
 * fork experience throughout the component tree.
 *
 * @example
 * ```tsx
 * <ForkProvider
 *   apiBaseUrl="http://localhost:8080"
 *   providers={providers}
 *   onForkComplete={(result) => console.log('Forked:', result)}
 *   onNavigateToSession={(id) => router.push(`/session/${id}`)}
 * >
 *   <ChatMain ... />
 * </ForkProvider>
 * ```
 */
export const ForkProvider: React.FC<ForkProviderProps> = ({
  children,
  apiBaseUrl,
  authToken,
  providers = [],
  messages = [],
  onForkComplete,
  onNavigateToSession,
}) => {
  // Initialize the fork hook
  const forkHook = useFork({ baseUrl: apiBaseUrl, authToken })

  // Handle fork execution with callback
  const handleFork = useCallback(
    async (
      sessionId: string,
      forkPointMessageId: string,
      options: ForkOptions
    ): Promise<ForkResult> => {
      const result = await forkHook.forkSession(sessionId, forkPointMessageId, options)
      onForkComplete?.(result)
      return result
    },
    [forkHook.forkSession, onForkComplete]
  )

  // Handle navigation with dialog close
  const handleNavigate = useCallback(
    (sessionId: string) => {
      forkHook.closeForkDialog()
      onNavigateToSession?.(sessionId)
    },
    [forkHook.closeForkDialog, onNavigateToSession]
  )

  // Build context value
  const contextValue = useMemo<ForkContextValue>(
    () => ({
      isForkDialogOpen: forkHook.isForkDialogOpen,
      sessionToFork: forkHook.sessionToFork,
      forkPointMessage: forkHook.forkPointMessage,
      isForking: forkHook.isForking,
      forkError: forkHook.forkError,
      lastForkResult: forkHook.lastForkResult,
      forkInfo: forkHook.forkInfo,
      forkInfoLoading: forkHook.forkInfoLoading,
      sessionForks: forkHook.sessionForks,
      sessionForksLoading: forkHook.sessionForksLoading,
      openForkDialog: forkHook.openForkDialog,
      closeForkDialog: forkHook.closeForkDialog,
      getForkInfo: forkHook.getForkInfo,
      listSessionForks: forkHook.listSessionForks,
      clearForkError: forkHook.clearForkError,
    }),
    [
      forkHook.isForkDialogOpen,
      forkHook.sessionToFork,
      forkHook.forkPointMessage,
      forkHook.isForking,
      forkHook.forkError,
      forkHook.lastForkResult,
      forkHook.forkInfo,
      forkHook.forkInfoLoading,
      forkHook.sessionForks,
      forkHook.sessionForksLoading,
      forkHook.openForkDialog,
      forkHook.closeForkDialog,
      forkHook.getForkInfo,
      forkHook.listSessionForks,
      forkHook.clearForkError,
    ]
  )

  return (
    <ForkContext.Provider value={contextValue}>
      {children}

      {/* Render the fork dialog when a session is being forked */}
      {forkHook.sessionToFork && (
        <SessionForkDialog
          session={forkHook.sessionToFork}
          forkPointMessage={forkHook.forkPointMessage}
          messages={messages}
          providers={providers}
          isOpen={forkHook.isForkDialogOpen}
          onClose={forkHook.closeForkDialog}
          onFork={handleFork}
          onNavigateToSession={handleNavigate}
          isLoading={forkHook.isForking}
        />
      )}
    </ForkContext.Provider>
  )
}

// =============================================================================
// Context Hook
// =============================================================================

/**
 * useForkContext - Access the fork context
 *
 * Must be used within a ForkProvider component.
 *
 * @example
 * ```tsx
 * function MessageComponent({ message }: { message: Message }) {
 *   const { openForkDialog } = useForkContext()
 *
 *   return (
 *     <button onClick={() => openForkDialog(session, message)}>
 *       Fork here
 *     </button>
 *   )
 * }
 * ```
 */
export function useForkContext(): ForkContextValue {
  const context = useContext(ForkContext)
  if (context === undefined) {
    throw new Error('useForkContext must be used within a ForkProvider')
  }
  return context
}

// =============================================================================
// Optional Context Hook (Non-throwing)
// =============================================================================

/**
 * useForkContextOptional - Access the fork context without throwing
 *
 * Returns undefined if not within a ForkProvider.
 * Useful for components that work with or without fork functionality.
 */
export function useForkContextOptional(): ForkContextValue | undefined {
  return useContext(ForkContext)
}

export default ForkProvider
