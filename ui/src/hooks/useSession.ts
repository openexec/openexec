/**
 * Session Management Hook
 *
 * Provides session lifecycle operations:
 * - Create, load, update, archive sessions
 * - Session forking for branching conversations
 * - Session filtering and listing
 * - WebSocket subscription management
 * - Project workspace discovery
 *
 * @module hooks/useSession
 */

import { useCallback, useEffect, useState } from 'react'
import type {
  Session,
  SessionListItem,
  SessionFilters,
  CreateSessionParams,
  ProjectInfo,
} from '../types'

// =============================================================================
// API Configuration
// =============================================================================

export interface SessionApiConfig {
  /** Base URL for REST API */
  baseUrl: string
  /** Optional auth token */
  authToken?: string
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseSessionReturn {
  /** List of sessions */
  sessions: SessionListItem[]
  /** Whether sessions are loading */
  sessionsLoading: boolean
  /** Sessions loading error */
  sessionsError: string | undefined
  /** List of available projects */
  projects: ProjectInfo[]
  /** Whether projects are loading */
  projectsLoading: boolean
  /** Projects loading error */
  projectsError: string | undefined
  /** Current active session */
  currentSession: Session | undefined
  /** Whether current session is loading */
  currentSessionLoading: boolean
  /** Current session loading error */
  currentSessionError: string | undefined
  /** Current session filters */
  filters: SessionFilters
  /** Fetch sessions with optional filters */
  fetchSessions: (filters?: SessionFilters) => Promise<void>
  /** Fetch available projects */
  fetchProjects: () => Promise<void>
  /** Create a new session */
  createSession: (params: CreateSessionParams) => Promise<Session>
  /** Initialize a new project */
  initProject: (name: string, path: string) => Promise<void>
  /** Load a session by ID */
  loadSession: (sessionId: string) => Promise<Session>
  /** Update session title */
  updateSessionTitle: (sessionId: string, title: string) => Promise<void>
  /** Archive a session */
  archiveSession: (sessionId: string) => Promise<void>
  /** Delete a session (soft delete) */
  deleteSession: (sessionId: string) => Promise<void>
  /** Fork a session at a specific message */
  forkSession: (sessionId: string, forkPointMessageId?: string) => Promise<Session>
  /** Update session filters */
  setFilters: (filters: SessionFilters) => void
  /** Clear current session */
  clearCurrentSession: () => void
  /** Refresh current session */
  refreshCurrentSession: () => Promise<void>
}

// =============================================================================
// HTTP Client Helper
// =============================================================================

async function apiRequest<T>(
  url: string,
  options: RequestInit,
  authToken?: string
): Promise<T> {
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(authToken && { Authorization: `Bearer ${authToken}` }),
    ...options.headers,
  }

  const response = await fetch(url, {
    ...options,
    headers,
  })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(`API error ${response.status}: ${errorText}`)
  }

  // Handle empty responses
  const text = await response.text()
  if (!text) {
    return undefined as T
  }

  return JSON.parse(text) as T
}

// =============================================================================
// useSession Hook
// =============================================================================

export function useSession(config: SessionApiConfig): UseSessionReturn {
  const { baseUrl, authToken } = config

  // State
  const [sessions, setSessions] = useState<SessionListItem[]>([])
  const [sessionsLoading, setSessionsLoading] = useState(false)
  const [sessionsError, setSessionsError] = useState<string | undefined>()
  const [projects, setProjects] = useState<ProjectInfo[]>([])
  const [projectsLoading, setProjectsLoading] = useState(false)
  const [projectsError, setProjectsError] = useState<string | undefined>()
  const [currentSession, setCurrentSession] = useState<Session | undefined>()
  const [currentSessionLoading, setCurrentSessionLoading] = useState(false)
  const [currentSessionError, setCurrentSessionError] = useState<string | undefined>()
  const [filters, setFilters] = useState<SessionFilters>({})

  // Fetch projects
  const fetchProjects = useCallback(async () => {
    setProjectsLoading(true)
    setProjectsError(undefined)

    try {
      const url = `${baseUrl}/projects`
      const data = await apiRequest<ProjectInfo[]>(url, { method: 'GET' }, authToken)
      setProjects(data || [])
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch projects'
      setProjectsError(message)
    } finally {
      setProjectsLoading(false)
    }
  }, [baseUrl, authToken])

  // Fetch sessions
  const fetchSessions = useCallback(
    async (newFilters?: SessionFilters) => {
      setSessionsLoading(true)
      setSessionsError(undefined)

      const activeFilters = newFilters ?? filters

      try {
        const params = new URLSearchParams()
        if (activeFilters.projectPath) params.set('project_path', activeFilters.projectPath)
        if (activeFilters.status) params.set('status', activeFilters.status)
        if (activeFilters.search) params.set('search', activeFilters.search)
        if (activeFilters.sortBy) params.set('sort_by', activeFilters.sortBy)
        if (activeFilters.sortOrder) params.set('sort_order', activeFilters.sortOrder)

        const url = `${baseUrl}/sessions${params.toString() ? `?${params}` : ''}`
        const data = await apiRequest<SessionListItem[]>(url, { method: 'GET' }, authToken)

        setSessions(data || [])
        if (newFilters) {
          setFilters(newFilters)
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to fetch sessions'
        setSessionsError(message)
      } finally {
        setSessionsLoading(false)
      }
    },
    [baseUrl, authToken, filters]
  )

  // Create session
  const createSession = useCallback(
    async (params: CreateSessionParams): Promise<Session> => {
      const url = `${baseUrl}/sessions`
      const session = await apiRequest<Session>(
        url,
        {
          method: 'POST',
          body: JSON.stringify(params),
        },
        authToken
      )

      // Add to sessions list
      const listItem: SessionListItem = {
        id: session.id,
        title: session.title,
        provider: session.provider,
        model: session.model,
        status: session.status,
        messageCount: 0,
        totalCostUsd: 0,
        createdAt: session.createdAt,
        updatedAt: session.updatedAt,
      }
      setSessions((prev) => [listItem, ...prev])

      return session
    },
    [baseUrl, authToken]
  )

  // Initialize project
  const initProject = useCallback(
    async (name: string, path: string) => {
      const url = `${baseUrl}/projects/init`
      await apiRequest<any>(
        url,
        {
          method: 'POST',
          body: JSON.stringify({ name, path }),
        },
        authToken
      )
      // Refresh projects list
      await fetchProjects()
    },
    [baseUrl, authToken, fetchProjects]
  )

  // Load session
  const loadSession = useCallback(
    async (sessionId: string): Promise<Session> => {
      setCurrentSessionLoading(true)
      setCurrentSessionError(undefined)

      try {
        const url = `${baseUrl}/sessions/${sessionId}`
        const session = await apiRequest<Session>(url, { method: 'GET' }, authToken)

        setCurrentSession(session)
        return session
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to load session'
        setCurrentSessionError(message)
        throw err
      } finally {
        setCurrentSessionLoading(false)
      }
    },
    [baseUrl, authToken]
  )

  // Update session title
  const updateSessionTitle = useCallback(
    async (sessionId: string, title: string): Promise<void> => {
      const url = `${baseUrl}/sessions/${sessionId}`
      await apiRequest<void>(
        url,
        {
          method: 'PATCH',
          body: JSON.stringify({ title }),
        },
        authToken
      )

      // Update local state
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, title, updatedAt: new Date().toISOString() } : s
        )
      )

      if (currentSession?.id === sessionId) {
        setCurrentSession((prev) =>
          prev ? { ...prev, title, updatedAt: new Date().toISOString() } : prev
        )
      }
    },
    [baseUrl, authToken, currentSession?.id]
  )

  // Archive session
  const archiveSession = useCallback(
    async (sessionId: string): Promise<void> => {
      const url = `${baseUrl}/sessions/${sessionId}/archive`
      await apiRequest<void>(url, { method: 'POST' }, authToken)

      // Update local state
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId
            ? { ...s, status: 'archived' as const, updatedAt: new Date().toISOString() }
            : s
        )
      )

      if (currentSession?.id === sessionId) {
        setCurrentSession((prev) =>
          prev
            ? { ...prev, status: 'archived' as const, updatedAt: new Date().toISOString() }
            : prev
        )
      }
    },
    [baseUrl, authToken, currentSession?.id]
  )

  // Delete session
  const deleteSession = useCallback(
    async (sessionId: string): Promise<void> => {
      const url = `${baseUrl}/sessions/${sessionId}`
      await apiRequest<void>(url, { method: 'DELETE' }, authToken)

      // Update local state
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId
            ? { ...s, status: 'deleted' as const, updatedAt: new Date().toISOString() }
            : s
        )
      )

      if (currentSession?.id === sessionId) {
        setCurrentSession(undefined)
      }
    },
    [baseUrl, authToken, currentSession?.id]
  )

  // Fork session
  const forkSession = useCallback(
    async (sessionId: string, forkPointMessageId?: string): Promise<Session> => {
      const url = `${baseUrl}/sessions/${sessionId}/fork`
      const session = await apiRequest<Session>(
        url,
        {
          method: 'POST',
          body: JSON.stringify({ forkPointMessageId }),
        },
        authToken
      )

      // Add forked session to list
      const listItem: SessionListItem = {
        id: session.id,
        title: session.title,
        provider: session.provider,
        model: session.model,
        status: session.status,
        messageCount: 0, // Will be populated by backend
        totalCostUsd: 0,
        createdAt: session.createdAt,
        updatedAt: session.updatedAt,
      }
      setSessions((prev) => [listItem, ...prev])

      return session
    },
    [baseUrl, authToken]
  )

  // Clear current session
  const clearCurrentSession = useCallback(() => {
    setCurrentSession(undefined)
    setCurrentSessionError(undefined)
  }, [])

  // Refresh current session
  const refreshCurrentSession = useCallback(async () => {
    if (currentSession) {
      await loadSession(currentSession.id)
    }
  }, [currentSession, loadSession])

  // Load sessions from localStorage on mount
  useEffect(() => {
    try {
      const cached = localStorage.getItem('openexec-sessions')
      if (cached) {
        const { sessions: cachedSessions, timestamp } = JSON.parse(cached)
        const maxAge = 5 * 60 * 1000 // 5 minutes
        if (Date.now() - timestamp < maxAge) {
          setSessions(cachedSessions)
        }
      }
    } catch {
      // Ignore localStorage errors
    }
  }, [])

  // Persist sessions to localStorage
  useEffect(() => {
    if (sessions.length > 0) {
      try {
        localStorage.setItem(
          'openexec-sessions',
          JSON.stringify({
            sessions,
            timestamp: Date.now(),
          })
        )
      } catch {
        // Ignore localStorage errors
      }
    }
  }, [sessions])

  // Persist current session ID
  useEffect(() => {
    if (currentSession) {
      try {
        localStorage.setItem('openexec-current-session', currentSession.id)
      } catch {
        // Ignore localStorage errors
      }
    }
  }, [currentSession])

  return {
    sessions,
    sessionsLoading,
    sessionsError,
    projects,
    projectsLoading,
    projectsError,
    currentSession,
    currentSessionLoading,
    currentSessionError,
    filters,
    fetchSessions,
    fetchProjects,
    createSession,
    initProject,
    loadSession,
    updateSessionTitle,
    archiveSession,
    deleteSession,
    forkSession,
    setFilters,
    clearCurrentSession,
    refreshCurrentSession,
  }
}

export default useSession
