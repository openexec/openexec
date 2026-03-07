/**
 * Tests for useRestartApproval Hook
 * @module hooks/__tests__/useRestartApproval
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useRestartApproval } from '../useRestartApproval'
import type { RestartRequest } from '../../types/restart'

// Mock fetch
const mockFetch = vi.fn()
global.fetch = mockFetch

// Mock WebSocket
class MockWebSocket {
  static instances: MockWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((error: Event) => void) | null = null
  readyState = WebSocket.OPEN
  sentMessages: string[] = []

  constructor(public url: string) {
    MockWebSocket.instances.push(this)
    // Simulate async connection
    setTimeout(() => this.onopen?.(), 0)
  }

  send(data: string) {
    this.sentMessages.push(data)
  }

  close() {
    this.readyState = WebSocket.CLOSED
    this.onclose?.({ code: 1000, reason: 'Normal closure' } as CloseEvent)
  }

  simulateMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }
}

// Replace global WebSocket with mock
const originalWebSocket = global.WebSocket
beforeEach(() => {
  global.WebSocket = MockWebSocket as unknown as typeof WebSocket
  MockWebSocket.instances = []
})
afterEach(() => {
  global.WebSocket = originalWebSocket
  vi.clearAllMocks()
})

// Test data
const createMockRequest = (overrides: Partial<RestartRequest> = {}): RestartRequest => ({
  id: 'req-123',
  reason: 'code_change',
  description: 'Test restart request',
  requestedBy: 'agent-456',
  sessionId: 'session-789',
  status: 'pending',
  buildRequired: true,
  port: 8080,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  resumeEnabled: false,
  resumeOnStartup: false,
  ...overrides,
})

const mockApiResponse = (data: unknown, status = 200) => {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  })
}

const mockApiError = (message: string, status = 500) => {
  mockFetch.mockResolvedValueOnce({
    ok: false,
    status,
    text: () => Promise.resolve(message),
  })
}

describe('useRestartApproval', () => {
  const defaultConfig = {
    baseUrl: 'http://localhost:8080/api',
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('initial state', () => {
    it('starts with empty requests and loading state', async () => {
      mockApiResponse([])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      expect(result.current.requests).toEqual([])
      expect(result.current.loading).toBe(true)
      expect(result.current.error).toBeUndefined()
      expect(result.current.hasPendingRequests).toBe(false)
      expect(result.current.pendingCount).toBe(0)

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })
    })

    it('fetches requests on mount', async () => {
      const mockRequests = [createMockRequest()]
      mockApiResponse(mockRequests)

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/restart/requests',
        expect.objectContaining({ method: 'GET' })
      )
      expect(result.current.requests).toEqual(mockRequests)
      expect(result.current.hasPendingRequests).toBe(true)
      expect(result.current.pendingCount).toBe(1)
    })
  })

  describe('fetchRequests', () => {
    it('updates requests state on success', async () => {
      mockApiResponse([])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const newRequests = [
        createMockRequest({ id: 'req-1' }),
        createMockRequest({ id: 'req-2', status: 'approved' }),
      ]
      mockApiResponse(newRequests)

      await act(async () => {
        await result.current.fetchRequests()
      })

      expect(result.current.requests).toEqual(newRequests)
      expect(result.current.hasPendingRequests).toBe(true)
      expect(result.current.pendingCount).toBe(1)
    })

    it('sets error on failure', async () => {
      mockApiResponse([])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      mockApiError('Server error', 500)

      await act(async () => {
        await result.current.fetchRequests()
      })

      expect(result.current.error).toContain('Server error')
    })
  })

  describe('approveRequest', () => {
    it('sends approve request and updates local state', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      mockApiResponse(undefined, 200)

      await act(async () => {
        await result.current.approveRequest(request.id, 'Approved for testing')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/restart/requests/${request.id}/approve`,
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ reason: 'Approved for testing' }),
        })
      )

      const updated = result.current.requests.find((r) => r.id === request.id)
      expect(updated?.status).toBe('approved')
      expect(updated?.approvedAt).toBeDefined()
    })

    it('clears selected request after approval', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      act(() => {
        result.current.selectRequest(request)
      })

      expect(result.current.selectedRequest).toEqual(request)

      mockApiResponse(undefined, 200)

      await act(async () => {
        await result.current.approveRequest(request.id)
      })

      expect(result.current.selectedRequest).toBeUndefined()
    })

    it('throws error on approval failure', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      mockApiError('Approval failed', 400)

      // The function should throw when approval fails
      let thrownError: Error | undefined
      try {
        await act(async () => {
          await result.current.approveRequest(request.id)
        })
      } catch (err) {
        thrownError = err as Error
      }

      expect(thrownError).toBeDefined()
      expect(thrownError!.message).toContain('Approval failed')
    })
  })

  describe('rejectRequest', () => {
    it('sends reject request and updates local state', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      mockApiResponse(undefined, 200)

      await act(async () => {
        await result.current.rejectRequest(request.id, 'Not safe to restart')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/restart/requests/${request.id}/reject`,
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ reason: 'Not safe to restart' }),
        })
      )

      const updated = result.current.requests.find((r) => r.id === request.id)
      expect(updated?.status).toBe('rejected')
      expect(updated?.completedAt).toBeDefined()
    })
  })

  describe('cancelRequest', () => {
    it('sends cancel request and updates local state', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      mockApiResponse(undefined, 200)

      await act(async () => {
        await result.current.cancelRequest(request.id, 'User cancelled')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/restart/requests/${request.id}/cancel`,
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ reason: 'User cancelled' }),
        })
      )

      const updated = result.current.requests.find((r) => r.id === request.id)
      expect(updated?.status).toBe('cancelled')
      expect(updated?.completedAt).toBeDefined()
    })
  })

  describe('selectRequest', () => {
    it('sets and clears selected request', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      act(() => {
        result.current.selectRequest(request)
      })

      expect(result.current.selectedRequest).toEqual(request)

      act(() => {
        result.current.selectRequest(undefined)
      })

      expect(result.current.selectedRequest).toBeUndefined()
    })
  })

  describe('banner dismissal', () => {
    it('dismisses and clears banner state', async () => {
      mockApiResponse([])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.areBannersDismissed).toBe(false)

      act(() => {
        result.current.dismissBanners()
      })

      expect(result.current.areBannersDismissed).toBe(true)

      act(() => {
        result.current.clearDismissed()
      })

      expect(result.current.areBannersDismissed).toBe(false)
    })
  })

  describe('WebSocket integration', () => {
    it('connects to WebSocket when URL provided', async () => {
      mockApiResponse([])

      const config = {
        ...defaultConfig,
        wsUrl: 'ws://localhost:8080/ws',
      }

      renderHook(() => useRestartApproval(config))

      await waitFor(() => {
        expect(MockWebSocket.instances.length).toBe(1)
        expect(MockWebSocket.instances[0].url).toBe('ws://localhost:8080/ws')
      })
    })

    it('subscribes to restart events on connect', async () => {
      mockApiResponse([])

      const config = {
        ...defaultConfig,
        wsUrl: 'ws://localhost:8080/ws',
      }

      renderHook(() => useRestartApproval(config))

      await waitFor(() => {
        expect(MockWebSocket.instances.length).toBe(1)
      })

      // Wait for subscription message
      await waitFor(() => {
        const ws = MockWebSocket.instances[0]
        expect(ws.sentMessages.length).toBeGreaterThan(0)
        const subscribeMsg = JSON.parse(ws.sentMessages[0])
        expect(subscribeMsg.type).toBe('subscribe')
        expect(subscribeMsg.topic).toBe('restart')
      })
    })

    it('handles restart_request_created event', async () => {
      mockApiResponse([])

      const config = {
        ...defaultConfig,
        wsUrl: 'ws://localhost:8080/ws',
      }

      const { result } = renderHook(() => useRestartApproval(config))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const newRequest = createMockRequest({ id: 'new-req' })

      await waitFor(() => {
        expect(MockWebSocket.instances.length).toBe(1)
      })

      act(() => {
        MockWebSocket.instances[0].simulateMessage({
          type: 'restart_request_created',
          payload: newRequest,
        })
      })

      expect(result.current.requests).toContainEqual(newRequest)
      expect(result.current.areBannersDismissed).toBe(false)
    })

    it('handles restart_request_updated event', async () => {
      const existingRequest = createMockRequest()
      mockApiResponse([existingRequest])

      const config = {
        ...defaultConfig,
        wsUrl: 'ws://localhost:8080/ws',
      }

      const { result } = renderHook(() => useRestartApproval(config))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const updatedRequest = { ...existingRequest, status: 'approved' as const }

      await waitFor(() => {
        expect(MockWebSocket.instances.length).toBe(1)
      })

      act(() => {
        MockWebSocket.instances[0].simulateMessage({
          type: 'restart_request_updated',
          payload: updatedRequest,
        })
      })

      const found = result.current.requests.find((r) => r.id === existingRequest.id)
      expect(found?.status).toBe('approved')
    })

    it('handles restart_request_deleted event', async () => {
      const existingRequest = createMockRequest()
      mockApiResponse([existingRequest])

      const config = {
        ...defaultConfig,
        wsUrl: 'ws://localhost:8080/ws',
      }

      const { result } = renderHook(() => useRestartApproval(config))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
        expect(result.current.requests.length).toBe(1)
      })

      await waitFor(() => {
        expect(MockWebSocket.instances.length).toBe(1)
      })

      act(() => {
        MockWebSocket.instances[0].simulateMessage({
          type: 'restart_request_deleted',
          payload: { id: existingRequest.id },
        })
      })

      expect(result.current.requests).toEqual([])
    })
  })

  describe('polling fallback', () => {
    it('uses polling when no WebSocket URL provided', async () => {
      vi.useFakeTimers()
      mockApiResponse([])

      const config = {
        ...defaultConfig,
        pollInterval: 1000,
      }

      const { result } = renderHook(() => useRestartApproval(config))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(mockFetch).toHaveBeenCalledTimes(1)

      // Advance timer to trigger poll
      mockApiResponse([createMockRequest()])
      vi.advanceTimersByTime(1000)

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledTimes(2)
      })

      vi.useRealTimers()
    })
  })

  describe('runPreflightChecks', () => {
    it('fetches preflight checks', async () => {
      mockApiResponse([])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const preflightResult = {
        allPassed: true,
        checks: [
          { name: 'go_binary', description: 'Go available', passed: true, critical: true },
        ],
      }
      mockApiResponse(preflightResult)

      const preflight = await act(async () => {
        return result.current.runPreflightChecks('req-123')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/restart/requests/req-123/preflight',
        expect.objectContaining({ method: 'GET' })
      )
      expect(preflight).toEqual(preflightResult)
    })
  })

  describe('action loading state', () => {
    it('sets actionLoading to false after approval completes', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.actionLoading).toBe(false)

      // Mock a successful approval
      mockApiResponse(undefined, 200)

      await act(async () => {
        await result.current.approveRequest(request.id)
      })

      // After approval completes, actionLoading should be false
      expect(result.current.actionLoading).toBe(false)
    })

    it('sets actionLoading to false after approval fails', async () => {
      const request = createMockRequest()
      mockApiResponse([request])

      const { result } = renderHook(() => useRestartApproval(defaultConfig))

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.actionLoading).toBe(false)

      // Mock a failed approval
      mockApiError('Failed', 500)

      try {
        await act(async () => {
          await result.current.approveRequest(request.id)
        })
      } catch {
        // Expected to throw
      }

      // After approval fails, actionLoading should be false
      expect(result.current.actionLoading).toBe(false)
    })
  })
})
