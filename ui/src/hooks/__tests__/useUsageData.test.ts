/**
 * Tests for useUsageData hook
 *
 * @module hooks/__tests__/useUsageData.test
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useUsageData, type UseUsageDataConfig } from '../useUsageData'

// Mock fetch globally
const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

describe('useUsageData', () => {
  const defaultConfig: UseUsageDataConfig = {
    apiBaseUrl: '/api',
    autoFetch: false,
  }

  // Mock API responses
  const mockUsageSummary = {
    total_tokens_input: 15000,
    total_tokens_output: 7500,
    total_tokens: 22500,
    total_cost_usd: 0.80,
    total_requests: 18,
    successful_requests: 16,
    failed_requests: 2,
    average_duration_ms: 1500,
    by_provider: {
      openai: {
        provider: 'openai',
        total_tokens_input: 10000,
        total_tokens_output: 5000,
        total_cost_usd: 0.50,
        total_requests: 10,
      },
    },
  }

  const mockProviderUsage = {
    providers: [
      {
        provider: 'openai',
        model: 'gpt-4',
        session_count: 5,
        message_count: 50,
        total_tokens_input: 10000,
        total_tokens_output: 5000,
        total_cost_usd: 0.50,
      },
      {
        provider: 'anthropic',
        model: 'claude-3-opus',
        session_count: 3,
        message_count: 30,
        total_tokens_input: 5000,
        total_tokens_output: 2500,
        total_cost_usd: 0.30,
      },
    ],
    total: {
      session_count: 8,
      message_count: 80,
      total_tokens_input: 15000,
      total_tokens_output: 7500,
      total_tokens: 22500,
      total_cost_usd: 0.80,
    },
  }

  const mockToolCallStats = {
    total_requested: 25,
    total_approved: 20,
    total_rejected: 2,
    total_auto_approved: 10,
    total_completed: 18,
    total_failed: 2,
    by_tool: {
      read_file: 10,
      write_file: 8,
      run_shell_command: 7,
    },
  }

  const mockAuditLogs = {
    entries: [
      {
        id: 'entry-1',
        timestamp: '2024-01-01T10:00:00Z',
        event_type: 'llm.request_end',
        severity: 'info',
        tokens_input: 1000,
        tokens_output: 500,
        cost_usd: 0.01,
        duration_ms: 1000,
        success: true,
      },
      {
        id: 'entry-2',
        timestamp: '2024-01-01T10:05:00Z',
        event_type: 'llm.request_end',
        severity: 'info',
        tokens_input: 2000,
        tokens_output: 1000,
        cost_usd: 0.02,
        duration_ms: 1200,
        success: true,
      },
    ],
    total_count: 2,
    has_more: false,
    limit: 100,
    offset: 0,
  }

  const mockSessionUsage = {
    session_id: 'session-1',
    message_count: 10,
    tool_call_count: 5,
    total_tokens_input: 5000,
    total_tokens_output: 2500,
    total_tokens: 7500,
    total_cost_usd: 0.25,
    summary_count: 2,
    tokens_saved: 1000,
  }

  beforeEach(() => {
    mockFetch.mockReset()
  })

  describe('initial state', () => {
    it('should have null/empty initial values', () => {
      const { result } = renderHook(() => useUsageData(defaultConfig))

      expect(result.current.usage).toBeNull()
      expect(result.current.providers).toEqual([])
      expect(result.current.toolCalls).toBeNull()
      expect(result.current.tokenHistory).toEqual([])
      expect(result.current.costTimeline).toEqual([])
      expect(result.current.sessionUsage).toBeNull()
      expect(result.current.loading).toBe(false)
      expect(result.current.error).toBeNull()
      expect(result.current.lastUpdated).toBeNull()
    })
  })

  describe('fetchSummary', () => {
    it('should fetch usage summary successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockUsageSummary,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchSummary()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/usage/summary',
        expect.any(Object)
      )

      expect(result.current.usage).toBeDefined()
      expect(result.current.usage?.totalTokensInput).toBe(15000)
      expect(result.current.usage?.totalTokensOutput).toBe(7500)
      expect(result.current.usage?.totalCostUsd).toBe(0.80)
      expect(result.current.usage?.totalRequests).toBe(18)
    })

    it('should transform by_provider data correctly', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockUsageSummary,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchSummary()
      })

      expect(result.current.usage?.byProvider).toBeDefined()
      expect(result.current.usage?.byProvider?.openai).toEqual({
        provider: 'openai',
        totalTokensInput: 10000,
        totalTokensOutput: 5000,
        totalCostUsd: 0.50,
        totalRequests: 10,
      })
    })
  })

  describe('fetchProviders', () => {
    it('should fetch provider usage successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockProviderUsage,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchProviders()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/usage/providers',
        expect.any(Object)
      )

      expect(result.current.providers).toHaveLength(2)
      expect(result.current.providers[0].provider).toBe('openai')
      expect(result.current.providers[1].provider).toBe('anthropic')
    })

    it('should transform provider stats correctly', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockProviderUsage,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchProviders()
      })

      const openaiStats = result.current.providers[0]
      expect(openaiStats.totalTokensInput).toBe(10000)
      expect(openaiStats.totalTokensOutput).toBe(5000)
      expect(openaiStats.totalCostUsd).toBe(0.50)
      expect(openaiStats.totalRequests).toBe(50) // Uses message_count
    })
  })

  describe('fetchToolCalls', () => {
    it('should fetch tool call stats successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockToolCallStats,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchToolCalls()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/usage/tools',
        expect.any(Object)
      )

      expect(result.current.toolCalls).toBeDefined()
      expect(result.current.toolCalls?.totalRequested).toBe(25)
      expect(result.current.toolCalls?.totalApproved).toBe(20)
      expect(result.current.toolCalls?.byTool?.read_file).toBe(10)
    })
  })

  describe('fetchSessionUsage', () => {
    it('should fetch session usage successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockSessionUsage,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchSessionUsage('session-1')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/usage/sessions/session-1',
        expect.any(Object)
      )

      expect(result.current.sessionUsage).toBeDefined()
      expect(result.current.sessionUsage?.session_id).toBe('session-1')
      expect(result.current.sessionUsage?.message_count).toBe(10)
      expect(result.current.sessionUsage?.total_cost_usd).toBe(0.25)
    })
  })

  describe('fetchAuditLogs', () => {
    it('should fetch audit logs and transform to timeline data', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockAuditLogs,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchAuditLogs(100)
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/usage/audit-logs'),
        expect.any(Object)
      )

      // Token history should be populated
      expect(result.current.tokenHistory).toHaveLength(2)
      expect(result.current.tokenHistory[0].inputTokens).toBe(1000)
      expect(result.current.tokenHistory[1].inputTokens).toBe(3000) // Cumulative

      // Cost timeline should be populated
      expect(result.current.costTimeline).toHaveLength(2)
      expect(result.current.costTimeline[0].cumulativeCost).toBe(0.01)
      expect(result.current.costTimeline[1].cumulativeCost).toBe(0.03) // Cumulative
    })
  })

  describe('fetchAll', () => {
    it('should fetch all data in parallel', async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockUsageSummary,
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockProviderUsage,
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockToolCallStats,
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockAuditLogs,
        })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchAll()
      })

      expect(mockFetch).toHaveBeenCalledTimes(4)
      expect(result.current.usage).toBeDefined()
      expect(result.current.providers).toHaveLength(2)
      expect(result.current.toolCalls).toBeDefined()
      expect(result.current.lastUpdated).toBeDefined()
    })
  })

  describe('clear', () => {
    it('should clear all data', async () => {
      mockFetch
        .mockResolvedValueOnce({ ok: true, json: async () => mockUsageSummary })
        .mockResolvedValueOnce({ ok: true, json: async () => mockProviderUsage })
        .mockResolvedValueOnce({ ok: true, json: async () => mockToolCallStats })
        .mockResolvedValueOnce({ ok: true, json: async () => mockAuditLogs })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await act(async () => {
        await result.current.fetchAll()
      })

      expect(result.current.usage).toBeDefined()

      act(() => {
        result.current.clear()
      })

      expect(result.current.usage).toBeNull()
      expect(result.current.providers).toEqual([])
      expect(result.current.toolCalls).toBeNull()
      expect(result.current.tokenHistory).toEqual([])
      expect(result.current.costTimeline).toEqual([])
      expect(result.current.sessionUsage).toBeNull()
      expect(result.current.error).toBeNull()
      expect(result.current.lastUpdated).toBeNull()
    })
  })

  describe('not auto-fetch', () => {
    it('should not fetch on mount when autoFetch is false', () => {
      renderHook(() => useUsageData({ ...defaultConfig, autoFetch: false }))
      expect(mockFetch).not.toHaveBeenCalled()
    })
  })

  describe('error handling', () => {
    it('should handle fetch error in fetchSummary', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await expect(
        act(async () => {
          await result.current.fetchSummary()
        })
      ).rejects.toThrow()
    })

    it('should handle fetch error in fetchProviders', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await expect(
        act(async () => {
          await result.current.fetchProviders()
        })
      ).rejects.toThrow()
    })

    it('should handle fetch error in fetchToolCalls', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      })

      const { result } = renderHook(() => useUsageData(defaultConfig))

      await expect(
        act(async () => {
          await result.current.fetchToolCalls()
        })
      ).rejects.toThrow()
    })
  })

  describe('query string building', () => {
    it('should include since parameter when provided', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockUsageSummary,
      })

      const since = new Date('2024-01-01T00:00:00Z')

      const { result } = renderHook(() =>
        useUsageData({ ...defaultConfig, since })
      )

      await act(async () => {
        await result.current.fetchSummary()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('since='),
        expect.any(Object)
      )
    })

    it('should include sessionId parameter when provided', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockUsageSummary,
      })

      const { result } = renderHook(() =>
        useUsageData({ ...defaultConfig, sessionId: 'session-123' })
      )

      await act(async () => {
        await result.current.fetchSummary()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('session_id=session-123'),
        expect.any(Object)
      )
    })
  })
})
