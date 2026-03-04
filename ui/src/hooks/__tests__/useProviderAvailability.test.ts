/**
 * Tests for useProviderAvailability hook
 *
 * @module hooks/__tests__/useProviderAvailability.test
 */

import { renderHook, waitFor, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  useProviderAvailability,
  clearProviderAvailabilityCache,
  type ProviderStatus,
  type ModelInfoResponse,
} from '../useProviderAvailability'

// =============================================================================
// Mock Data
// =============================================================================

const mockProviderStatus: ProviderStatus[] = [
  {
    name: 'openai',
    available: true,
    reason: 'configured and ready',
    models: ['gpt-4o', 'gpt-4o-mini'],
  },
  {
    name: 'anthropic',
    available: true,
    reason: 'configured and ready',
    models: ['claude-3-5-sonnet-20241022'],
  },
  {
    name: 'gemini',
    available: false,
    reason: 'API key not configured',
    models: [],
  },
]

const mockModels: ModelInfoResponse[] = [
  {
    id: 'gpt-4o',
    name: 'GPT-4o',
    provider: 'openai',
    capabilities: {
      maxContextTokens: 128000,
      maxOutputTokens: 16384,
      toolUse: true,
      streaming: true,
      vision: true,
    },
    pricePerMInputTokens: 2.5,
    pricePerMOutputTokens: 10.0,
  },
  {
    id: 'gpt-4o-mini',
    name: 'GPT-4o Mini',
    provider: 'openai',
    capabilities: {
      maxContextTokens: 128000,
      maxOutputTokens: 16384,
      toolUse: true,
      streaming: true,
      vision: true,
    },
    pricePerMInputTokens: 0.15,
    pricePerMOutputTokens: 0.6,
  },
  {
    id: 'claude-3-5-sonnet-20241022',
    name: 'Claude 3.5 Sonnet',
    provider: 'anthropic',
    capabilities: {
      maxContextTokens: 200000,
      maxOutputTokens: 8192,
      toolUse: true,
      streaming: true,
      vision: true,
    },
    pricePerMInputTokens: 3.0,
    pricePerMOutputTokens: 15.0,
  },
]

// =============================================================================
// Mock Setup
// =============================================================================

const mockFetch = vi.fn()

beforeEach(() => {
  clearProviderAvailabilityCache()
  vi.stubGlobal('fetch', mockFetch)
  vi.useFakeTimers()
})

afterEach(() => {
  vi.clearAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// Helper to create mock response
function createMockResponse<T>(data: T, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(data),
  } as Response
}

// =============================================================================
// Tests
// =============================================================================

describe('useProviderAvailability', () => {
  describe('initial fetch', () => {
    it('should fetch provider availability on mount', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      // Initially loading
      expect(result.current.loading).toBe(true)

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      // Should have fetched both status and models
      expect(mockFetch).toHaveBeenCalledTimes(2)
      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/providers',
        expect.any(Object)
      )
      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/models',
        expect.any(Object)
      )
    })

    it('should transform provider status to ProviderInfo', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      // Check provider count
      expect(result.current.providers).toHaveLength(3)

      // Check OpenAI provider
      const openai = result.current.providers.find((p) => p.id === 'openai')
      expect(openai).toBeDefined()
      expect(openai?.isAvailable).toBe(true)
      expect(openai?.name).toBe('OpenAI')
      expect(openai?.models).toHaveLength(2)

      // Check Anthropic provider
      const anthropic = result.current.providers.find((p) => p.id === 'anthropic')
      expect(anthropic).toBeDefined()
      expect(anthropic?.isAvailable).toBe(true)
      expect(anthropic?.name).toBe('Anthropic')

      // Check Gemini provider (unavailable)
      const gemini = result.current.providers.find((p) => p.id === 'gemini')
      expect(gemini).toBeDefined()
      expect(gemini?.isAvailable).toBe(false)
      expect(gemini?.statusMessage).toBe('API key not configured')
    })

    it('should handle API errors gracefully', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.error).toBe('Network error')
      expect(result.current.providers).toHaveLength(0)
    })

    it('should handle non-OK response status', async () => {
      mockFetch.mockResolvedValueOnce(createMockResponse('Not found', 404))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.error).toContain('API error 404')
    })

    it('should continue if models endpoint fails', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockRejectedValueOnce(new Error('Models endpoint not found'))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      // Should still have providers, just without detailed model info
      expect(result.current.providers).toHaveLength(3)
      expect(result.current.error).toBeUndefined()
    })
  })

  describe('caching', () => {
    it('should use cached data within TTL', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      const { result, rerender } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
          cacheTTL: 30000,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const fetchCount = mockFetch.mock.calls.length

      // Rerender should use cache
      rerender()

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      // No additional fetch calls
      expect(mockFetch.mock.calls.length).toBe(fetchCount)
    })

    it('should refetch after cache expires', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      const { result, unmount } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
          cacheTTL: 1000,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const initialFetchCount = mockFetch.mock.calls.length
      unmount()

      // Clear cache and advance time
      clearProviderAvailabilityCache()

      // New render should fetch again
      const { result: result2 } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
          cacheTTL: 1000,
        })
      )

      await waitFor(() => {
        expect(result2.current.loading).toBe(false)
      })

      expect(mockFetch.mock.calls.length).toBeGreaterThan(initialFetchCount)
    })
  })

  describe('polling', () => {
    it('should poll when enabled', async () => {
      mockFetch
        .mockResolvedValue(createMockResponse(mockProviderStatus))

      const pollingInterval = 5000

      renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: true,
          pollingInterval,
        })
      )

      // Wait for initial fetch
      await act(async () => { await vi.runOnlyPendingTimersAsync() })

      const initialCalls = mockFetch.mock.calls.length

      // Clear cache to force refetch
      clearProviderAvailabilityCache()

      // Advance by polling interval
      await act(async () => {
        vi.advanceTimersByTime(pollingInterval)
        await act(async () => { await vi.runOnlyPendingTimersAsync() })
      })

      // Should have made additional calls
      expect(mockFetch.mock.calls.length).toBeGreaterThan(initialCalls)
    })

    it('should not poll when disabled', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      // Wait for initial fetch
      await act(async () => {
        await act(async () => { await vi.runOnlyPendingTimersAsync() })
      })

      const initialCalls = mockFetch.mock.calls.length

      // Advance time significantly
      await act(async () => {
        vi.advanceTimersByTime(120000)
      })

      // No additional calls
      expect(mockFetch.mock.calls.length).toBe(initialCalls)
    })
  })

  describe('refresh', () => {
    it('should bypass cache when calling refresh', async () => {
      mockFetch
        .mockResolvedValue(createMockResponse(mockProviderStatus))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
          cacheTTL: 60000,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const initialCalls = mockFetch.mock.calls.length

      // Manual refresh should bypass cache
      await act(async () => {
        await result.current.refresh()
      })

      expect(mockFetch.mock.calls.length).toBeGreaterThan(initialCalls)
    })
  })

  describe('helper methods', () => {
    beforeEach(async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))
    })

    it('should return correct value for isProviderAvailable', async () => {
      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.isProviderAvailable('openai')).toBe(true)
      expect(result.current.isProviderAvailable('anthropic')).toBe(true)
      expect(result.current.isProviderAvailable('gemini')).toBe(false)
      expect(result.current.isProviderAvailable('unknown')).toBe(false)
    })

    it('should return correct value for getProviderStatus', async () => {
      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.getProviderStatus('openai')).toBeUndefined()
      expect(result.current.getProviderStatus('gemini')).toBe('API key not configured')
    })

    it('should return models for getProviderModels', async () => {
      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      const openaiModels = result.current.getProviderModels('openai')
      expect(openaiModels).toHaveLength(2)
      expect(openaiModels[0].id).toBe('gpt-4o')

      const unknownModels = result.current.getProviderModels('unknown')
      expect(unknownModels).toHaveLength(0)
    })

    it('should compute hasAvailableProviders correctly', async () => {
      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.hasAvailableProviders).toBe(true)
    })

    it('should list availableProviderIds correctly', async () => {
      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.availableProviderIds).toContain('openai')
      expect(result.current.availableProviderIds).toContain('anthropic')
      expect(result.current.availableProviderIds).not.toContain('gemini')
    })
  })

  describe('authentication', () => {
    it('should include auth token in requests when provided', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          authToken: 'test-token-123',
          enablePolling: false,
        })
      )

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalled()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token-123',
          }),
        })
      )
    })
  })

  describe('lastUpdated', () => {
    it('should update lastUpdated timestamp after successful fetch', async () => {
      mockFetch
        .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
        .mockResolvedValueOnce(createMockResponse(mockModels))

      const { result } = renderHook(() =>
        useProviderAvailability({
          baseUrl: 'http://localhost:8080',
          enablePolling: false,
        })
      )

      expect(result.current.lastUpdated).toBeUndefined()

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.lastUpdated).toBeInstanceOf(Date)
    })
  })
})
