/**
 * Tests for useRollback hook
 * @module hooks/__tests__/useRollback
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useRollback } from '../useRollback'
import type { BackupMetadata } from '../../types/backup'

// Mock backup data
const createMockBackup = (overrides: Partial<BackupMetadata> = {}): BackupMetadata => ({
  id: 'backup-123',
  originalPath: '/Users/test/project/src/test.ts',
  backupPath: '/backups/backup-123',
  checksum: 'abc123def456',
  size: 1024,
  createdAt: new Date().toISOString(),
  fileMode: 0o644,
  sessionId: 'session-456',
  restored: false,
  ...overrides,
})

// Mock fetch
const createMockFetch = (responses: Record<string, unknown>) => {
  return vi.fn((url: string, options?: RequestInit) => {
    const urlPath = new URL(url).pathname

    for (const [pattern, response] of Object.entries(responses)) {
      if (urlPath.includes(pattern)) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(response),
        })
      }
    }

    return Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'Not found' }),
    })
  })
}

describe('useRollback', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  describe('initial state', () => {
    it('starts with idle status', () => {
      const mockFetch = createMockFetch({})
      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      expect(result.current.state.status).toBe('idle')
      expect(result.current.isLoading).toBe(false)
      expect(result.current.isModalOpen).toBe(false)
      expect(result.current.selectedBackup).toBeUndefined()
    })
  })

  describe('initiateRollback', () => {
    it('opens modal and fetches diff', async () => {
      const mockBackup = createMockBackup()
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        currentChecksum: 'xyz789',
        isDifferent: true,
        currentSize: 2048,
        diff: '--- a/test.ts\n+++ b/test.ts\n@@ -1,1 +1,1 @@\n-old\n+new',
        isBinary: false,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      expect(result.current.isModalOpen).toBe(true)
      expect(result.current.selectedBackup).toEqual(mockBackup)
      expect(result.current.state.status).toBe('confirming')
      expect(result.current.currentFileExists).toBe(true)
      expect(result.current.currentFileSize).toBe(2048)
    })

    it('sets error state on API failure', async () => {
      const mockBackup = createMockBackup()
      const mockFetch = vi.fn(() =>
        Promise.resolve({
          ok: false,
          json: () => Promise.resolve({ error: 'Server error' }),
        })
      )

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      expect(result.current.state.status).toBe('error')
      expect(result.current.error).toBe('Server error')
    })

    it('generates warnings for old backups', async () => {
      const oldDate = new Date()
      oldDate.setDate(oldDate.getDate() - 10) // 10 days ago

      const mockBackup = createMockBackup({
        createdAt: oldDate.toISOString(),
      })

      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        currentSize: 1024,
        isBinary: false,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      expect(result.current.warnings).toContain('This backup is older than 7 days')
    })

    it('generates warning for previously restored backup', async () => {
      const mockBackup = createMockBackup({ restored: true })
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        currentSize: 1024,
        isBinary: false,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      expect(result.current.warnings).toContain('This backup was previously restored')
    })
  })

  describe('confirmRollback', () => {
    it('executes restore and updates state on success', async () => {
      const mockBackup = createMockBackup()
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        currentSize: 1024,
        isBinary: false,
      }
      const mockRestoreResponse = {
        success: true,
        backupId: mockBackup.id,
        filePath: mockBackup.originalPath,
        checksumVerified: true,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
        '/restore': mockRestoreResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      // First initiate
      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      // Then confirm
      await act(async () => {
        const restoreResult = await result.current.confirmRollback()
        expect(restoreResult.success).toBe(true)
      })

      expect(result.current.state.status).toBe('success')
    })

    it('sets error state on restore failure', async () => {
      const mockBackup = createMockBackup()
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        currentSize: 1024,
        isBinary: false,
      }

      const mockFetch = vi.fn((url: string) => {
        if (url.includes('/diff')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockDiffResponse),
          })
        }
        if (url.includes('/restore')) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                success: false,
                backupId: mockBackup.id,
                filePath: mockBackup.originalPath,
                error: 'Checksum mismatch',
                checksumVerified: false,
              }),
          })
        }
        return Promise.resolve({
          ok: false,
          json: () => Promise.resolve({ error: 'Not found' }),
        })
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      await act(async () => {
        const restoreResult = await result.current.confirmRollback()
        expect(restoreResult.success).toBe(false)
      })

      expect(result.current.state.status).toBe('error')
      expect(result.current.error).toBe('Checksum mismatch')
    })

    it('throws error when no backup selected', async () => {
      const mockFetch = createMockFetch({})

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await expect(
        act(async () => {
          await result.current.confirmRollback()
        })
      ).rejects.toThrow('No backup selected')
    })
  })

  describe('cancelRollback', () => {
    it('closes modal and resets state', async () => {
      const mockBackup = createMockBackup()
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: false,
        isBinary: false,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      expect(result.current.isModalOpen).toBe(true)

      act(() => {
        result.current.cancelRollback()
      })

      expect(result.current.isModalOpen).toBe(false)
      expect(result.current.state.status).toBe('idle')
      expect(result.current.selectedBackup).toBeUndefined()
    })
  })

  describe('closeModal', () => {
    it('closes modal without resetting state on success', async () => {
      const mockBackup = createMockBackup()
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        isBinary: false,
      }
      const mockRestoreResponse = {
        success: true,
        backupId: mockBackup.id,
        filePath: mockBackup.originalPath,
        checksumVerified: true,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
        '/restore': mockRestoreResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      await act(async () => {
        await result.current.confirmRollback()
      })

      act(() => {
        result.current.closeModal()
      })

      expect(result.current.isModalOpen).toBe(false)
      // State should remain success for display
      expect(result.current.state.status).toBe('success')
    })
  })

  describe('reset', () => {
    it('fully resets all state', async () => {
      const mockBackup = createMockBackup()
      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        currentSize: 2048,
        diff: '--- a/test.ts\n+++ b/test.ts',
        isBinary: false,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      act(() => {
        result.current.reset()
      })

      expect(result.current.isModalOpen).toBe(false)
      expect(result.current.state.status).toBe('idle')
      expect(result.current.selectedBackup).toBeUndefined()
      expect(result.current.diff).toBeUndefined()
      expect(result.current.diffStats).toBeUndefined()
      expect(result.current.warnings).toHaveLength(0)
      expect(result.current.currentFileExists).toBe(true)
      expect(result.current.currentFileSize).toBeUndefined()
    })
  })

  describe('diff parsing', () => {
    it('parses unified diff format correctly', async () => {
      const mockBackup = createMockBackup()
      const unifiedDiff = `--- a/src/test.ts
+++ b/src/test.ts
@@ -1,3 +1,4 @@
 const foo = 1
-const bar = 2
+const bar = 3
+const baz = 4
 export { foo }`

      const mockDiffResponse = {
        backupId: mockBackup.id,
        currentFileExists: true,
        isDifferent: true,
        currentSize: 1024,
        diff: unifiedDiff,
        isBinary: false,
      }

      const mockFetch = createMockFetch({
        '/diff': mockDiffResponse,
      })

      const { result } = renderHook(() =>
        useRollback({ baseUrl: 'http://localhost:8000', fetchFn: mockFetch })
      )

      await act(async () => {
        await result.current.initiateRollback(mockBackup)
      })

      expect(result.current.diff).toBeDefined()
      expect(result.current.diff?.files).toHaveLength(1)
      expect(result.current.diff?.files[0].oldName).toBe('src/test.ts')
      expect(result.current.diffStats?.additions).toBe(2)
      expect(result.current.diffStats?.deletions).toBe(1)
    })
  })
})
