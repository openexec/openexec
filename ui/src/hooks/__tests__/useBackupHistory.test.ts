/**
 * Tests for useBackupHistory hook
 * @module hooks/__tests__/useBackupHistory
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useBackupHistory } from '../useBackupHistory'
import type { BackupMetadata, BackupStats } from '../../types/backup'

// Mock backup data
const createMockBackup = (
  id: string,
  path: string,
  overrides: Partial<BackupMetadata> = {}
): BackupMetadata => ({
  id,
  originalPath: path,
  backupPath: `/backups/${id}`,
  checksum: `checksum-${id}`,
  size: 1024,
  createdAt: new Date().toISOString(),
  fileMode: 0o644,
  sessionId: 'session-456',
  restored: false,
  ...overrides,
})

const createMockStats = (): BackupStats => ({
  totalBackups: 10,
  totalFiles: 5,
  totalSizeBytes: 10240,
  oldestBackup: new Date(Date.now() - 86400000).toISOString(),
  newestBackup: new Date().toISOString(),
})

// Mock fetch helper
const createMockFetch = (response: unknown, options: { fail?: boolean; delay?: number } = {}) => {
  return vi.fn(() => {
    const promise = new Promise((resolve) => {
      const result = {
        ok: !options.fail,
        json: () => Promise.resolve(response),
      }
      if (options.delay) {
        setTimeout(() => resolve(result), options.delay)
      } else {
        resolve(result)
      }
    })
    return promise
  })
}

describe('useBackupHistory', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  describe('initial state', () => {
    it('starts with empty backups and loading state', async () => {
      const mockFetch = createMockFetch({
        backups: [],
        stats: createMockStats(),
        total: 0,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      expect(result.current.backups).toEqual([])
      expect(result.current.isInitialLoading).toBe(true)

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.isInitialLoading).toBe(false)
    })
  })

  describe('fetchBackups', () => {
    it('fetches and stores backups', async () => {
      const mockBackups = [
        createMockBackup('backup-1', '/path/to/file1.ts'),
        createMockBackup('backup-2', '/path/to/file2.ts'),
      ]

      const mockFetch = createMockFetch({
        backups: mockBackups,
        stats: createMockStats(),
        total: 2,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.backups).toHaveLength(2)
      expect(result.current.backups[0].id).toBe('backup-1')
    })

    it('applies file path filter', async () => {
      const mockFetch = createMockFetch({
        backups: [createMockBackup('backup-1', '/path/to/file1.ts')],
        stats: createMockStats(),
        total: 1,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.fetchBackups({ filePath: '/path/to/file1.ts' })
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('filePath=%2Fpath%2Fto%2Ffile1.ts'),
        expect.anything()
      )
    })

    it('applies session ID filter', async () => {
      const mockFetch = createMockFetch({
        backups: [],
        stats: createMockStats(),
        total: 0,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.fetchBackups({ sessionId: 'session-123' })
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('sessionId=session-123'),
        expect.anything()
      )
    })

    it('sets error on fetch failure', async () => {
      const mockFetch = createMockFetch({ error: 'Server error' }, { fail: true })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.error).toBe('Server error')
    })
  })

  describe('refresh', () => {
    it('refreshes the backup list', async () => {
      const mockBackups = [createMockBackup('backup-1', '/path/to/file1.ts')]
      const mockFetch = createMockFetch({
        backups: mockBackups,
        stats: createMockStats(),
        total: 1,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      // Initial fetch
      await act(async () => {
        await vi.runAllTimersAsync()
      })

      // Reset mock
      mockFetch.mockClear()

      // Refresh
      await act(async () => {
        await result.current.refresh()
      })

      expect(mockFetch).toHaveBeenCalled()
      expect(result.current.isRefreshing).toBe(false)
    })
  })

  describe('loadMore', () => {
    it('loads more backups with offset', async () => {
      const initialBackups = [createMockBackup('backup-1', '/path/to/file1.ts')]
      const moreBackups = [createMockBackup('backup-2', '/path/to/file2.ts')]

      let callCount = 0
      const mockFetch = vi.fn(() => {
        callCount++
        if (callCount === 1) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                backups: initialBackups,
                stats: createMockStats(),
                total: 2,
                hasMore: true,
              }),
          })
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              backups: moreBackups,
              stats: createMockStats(),
              total: 2,
              hasMore: false,
            }),
        })
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      // Initial fetch
      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.backups).toHaveLength(1)
      expect(result.current.hasMore).toBe(true)

      // Load more
      await act(async () => {
        await result.current.loadMore()
      })

      expect(result.current.backups).toHaveLength(2)
      expect(result.current.hasMore).toBe(false)
    })

    it('does not load more if hasMore is false', async () => {
      const mockBackups = [createMockBackup('backup-1', '/path/to/file1.ts')]
      const mockFetch = createMockFetch({
        backups: mockBackups,
        stats: createMockStats(),
        total: 1,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      const initialCallCount = mockFetch.mock.calls.length

      await act(async () => {
        await result.current.loadMore()
      })

      // Should not have made another call
      expect(mockFetch.mock.calls.length).toBe(initialCallCount)
    })
  })

  describe('deleteBackup', () => {
    it('deletes backup and updates list', async () => {
      const mockBackups = [
        createMockBackup('backup-1', '/path/to/file1.ts'),
        createMockBackup('backup-2', '/path/to/file2.ts'),
      ]

      let callCount = 0
      const mockFetch = vi.fn((url: string, options?: RequestInit) => {
        callCount++
        if (callCount === 1) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                backups: mockBackups,
                stats: createMockStats(),
                total: 2,
                hasMore: false,
              }),
          })
        }
        // Delete call
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true }),
        })
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.backups).toHaveLength(2)

      await act(async () => {
        const success = await result.current.deleteBackup('backup-1')
        expect(success).toBe(true)
      })

      expect(result.current.backups).toHaveLength(1)
      expect(result.current.backups[0].id).toBe('backup-2')
    })

    it('sets error on delete failure', async () => {
      const mockBackups = [createMockBackup('backup-1', '/path/to/file1.ts')]

      let callCount = 0
      const mockFetch = vi.fn(() => {
        callCount++
        if (callCount === 1) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                backups: mockBackups,
                stats: createMockStats(),
                total: 1,
                hasMore: false,
              }),
          })
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: false, error: 'Delete failed' }),
        })
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      await act(async () => {
        const success = await result.current.deleteBackup('backup-1')
        expect(success).toBe(false)
      })

      expect(result.current.error).toBe('Delete failed')
    })
  })

  describe('pruneBackups', () => {
    it('prunes old backups and refreshes', async () => {
      const mockBackups = [createMockBackup('backup-1', '/path/to/file1.ts')]

      let callCount = 0
      const mockFetch = vi.fn((url: string) => {
        callCount++
        // Check if this is a prune call
        if (url.includes('/prune')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ pruned: 5 }),
          })
        }
        // List backups call
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              backups: mockBackups,
              stats: createMockStats(),
              total: 1,
              hasMore: false,
            }),
        })
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      await act(async () => {
        const pruned = await result.current.pruneBackups()
        expect(pruned).toBe(5)
      })
    })
  })

  describe('clearError', () => {
    it('clears error state', async () => {
      const mockFetch = createMockFetch({ error: 'Server error' }, { fail: true })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.error).toBeDefined()

      act(() => {
        result.current.clearError()
      })

      expect(result.current.error).toBeUndefined()
    })
  })

  describe('real-time updates', () => {
    it('adds backup to list', async () => {
      const mockFetch = createMockFetch({
        backups: [],
        stats: createMockStats(),
        total: 0,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.backups).toHaveLength(0)

      const newBackup = createMockBackup('backup-new', '/path/to/new.ts')

      act(() => {
        result.current.addBackup(newBackup)
      })

      expect(result.current.backups).toHaveLength(1)
      expect(result.current.backups[0].id).toBe('backup-new')
    })

    it('removes backup from list', async () => {
      const mockBackups = [createMockBackup('backup-1', '/path/to/file1.ts')]
      const mockFetch = createMockFetch({
        backups: mockBackups,
        stats: createMockStats(),
        total: 1,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.backups).toHaveLength(1)

      act(() => {
        result.current.removeBackup('backup-1')
      })

      expect(result.current.backups).toHaveLength(0)
    })

    it('updates backup in list', async () => {
      const mockBackups = [
        createMockBackup('backup-1', '/path/to/file1.ts', { restored: false }),
      ]
      const mockFetch = createMockFetch({
        backups: mockBackups,
        stats: createMockStats(),
        total: 1,
        hasMore: false,
      })

      const { result } = renderHook(() =>
        useBackupHistory({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      expect(result.current.backups[0].restored).toBe(false)

      act(() => {
        result.current.updateBackup({
          ...result.current.backups[0],
          restored: true,
          restoredAt: new Date().toISOString(),
        })
      })

      expect(result.current.backups[0].restored).toBe(true)
    })
  })

  describe('auto-refresh', () => {
    it('refreshes at interval when enabled', async () => {
      const mockFetch = vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              backups: [],
              stats: createMockStats(),
              total: 0,
              hasMore: false,
            }),
        })
      )

      renderHook(() =>
        useBackupHistory({
          baseUrl: 'http://localhost:8000',
          fetchFn: mockFetch,
          autoRefresh: true,
          refreshInterval: 5000,
        })
      )

      // Wait for initial fetch
      await act(async () => {
        await Promise.resolve()
      })

      const initialCallCount = mockFetch.mock.calls.length
      expect(initialCallCount).toBeGreaterThanOrEqual(1)

      // Advance timer by refresh interval
      act(() => {
        vi.advanceTimersByTime(5000)
      })

      // Wait for refresh fetch
      await act(async () => {
        await Promise.resolve()
      })

      expect(mockFetch.mock.calls.length).toBeGreaterThan(initialCallCount)
    })

    it('does not auto-refresh when disabled', async () => {
      const mockFetch = createMockFetch({
        backups: [],
        stats: createMockStats(),
        total: 0,
        hasMore: false,
      })

      renderHook(() =>
        useBackupHistory({
          baseUrl: 'http://localhost:8000',
          fetchFn: mockFetch,
          autoRefresh: false,
          refreshInterval: 5000,
        })
      )

      await act(async () => {
        await vi.runAllTimersAsync()
      })

      const initialCallCount = mockFetch.mock.calls.length

      await act(async () => {
        vi.advanceTimersByTime(10000)
        await vi.runAllTimersAsync()
      })

      expect(mockFetch.mock.calls.length).toBe(initialCallCount)
    })
  })
})
