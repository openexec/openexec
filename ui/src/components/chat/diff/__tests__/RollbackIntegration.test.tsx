/**
 * Tests for RollbackIntegration Component
 * Integration layer that combines BackupHistoryPanel and RollbackModal
 * @module components/chat/diff/__tests__/RollbackIntegration
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { BackupMetadata, BackupStats } from '../../../../types/backup'

// Mock the hooks
vi.mock('../../../../hooks/useRollback', () => ({
  useRollback: vi.fn(),
}))

vi.mock('../../../../hooks/useBackupHistory', () => ({
  useBackupHistory: vi.fn(),
}))

// Import after mocking
import RollbackIntegration from '../RollbackIntegration'
import { useRollback } from '../../../../hooks/useRollback'
import { useBackupHistory } from '../../../../hooks/useBackupHistory'

// Mock backup data
const createMockBackup = (
  id: string,
  path: string,
  overrides: Partial<BackupMetadata> = {}
): BackupMetadata => ({
  id,
  originalPath: path,
  backupPath: `/backups/${id}`,
  checksum: `checksum-${id}-abc123def456abc123def456`,
  size: 1024,
  createdAt: new Date().toISOString(),
  fileMode: 0o644,
  sessionId: 'session-456',
  restored: false,
  ...overrides,
})

const createMockStats = (): BackupStats => ({
  totalBackups: 3,
  totalFiles: 2,
  totalSizeBytes: 3072,
  oldestBackup: new Date(Date.now() - 86400000).toISOString(),
  newestBackup: new Date().toISOString(),
})

// Default mock return values
const createDefaultRollbackHook = () => ({
  state: { status: 'idle' as const },
  isLoading: false,
  isModalOpen: false,
  selectedBackup: undefined,
  diff: undefined,
  diffStats: undefined,
  currentFileExists: true,
  currentFileSize: undefined,
  warnings: [],
  error: undefined,
  initiateRollback: vi.fn(),
  confirmRollback: vi.fn().mockResolvedValue({ success: true }),
  cancelRollback: vi.fn(),
  closeModal: vi.fn(),
  reset: vi.fn(),
})

const createDefaultBackupHistoryHook = (backups: BackupMetadata[] = []) => ({
  backups,
  stats: createMockStats(),
  isLoading: false,
  isInitialLoading: false,
  isRefreshing: false,
  error: undefined,
  hasMore: false,
  fetchBackups: vi.fn(),
  refresh: vi.fn(),
  loadMore: vi.fn(),
  deleteBackup: vi.fn().mockResolvedValue(true),
  pruneBackups: vi.fn().mockResolvedValue(0),
  clearError: vi.fn(),
  addBackup: vi.fn(),
  removeBackup: vi.fn(),
  updateBackup: vi.fn(),
})

describe('RollbackIntegration', () => {
  const mockOnRestoreComplete = vi.fn()
  const mockOnDeleteComplete = vi.fn()
  const mockOnError = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    // Setup default mocks
    ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue(createDefaultRollbackHook())
    ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(createDefaultBackupHistoryHook())
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  describe('rendering', () => {
    it('renders the backup history panel', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onRestoreComplete={mockOnRestoreComplete}
        />
      )

      expect(screen.getByText('Backup History')).toBeInTheDocument()
    })

    it('displays backups from hook', () => {
      const backups = [
        createMockBackup('backup-1', '/path/to/file1.ts'),
        createMockBackup('backup-2', '/path/to/file2.ts'),
      ]
      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onRestoreComplete={mockOnRestoreComplete}
        />
      )

      expect(screen.getByText('file1.ts')).toBeInTheDocument()
      expect(screen.getByText('file2.ts')).toBeInTheDocument()
    })

    it('uses custom title when provided', () => {
      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          title="File Rollbacks"
        />
      )

      expect(screen.getByText('File Rollbacks')).toBeInTheDocument()
    })

    it('hides stats when showStats is false', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          showStats={false}
        />
      )

      expect(screen.getByText('file.ts')).toBeInTheDocument()
      // Stats section should not be present
      expect(screen.queryByText('Total')).not.toBeInTheDocument()
    })

    it('hides filter when showFilter is false', () => {
      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          showFilter={false}
        />
      )

      expect(screen.getByText('Backup History')).toBeInTheDocument()
      expect(screen.queryByPlaceholderText('Search backups...')).not.toBeInTheDocument()
    })
  })

  describe('filtering by file path', () => {
    it('filters backups by file path', () => {
      const backups = [
        createMockBackup('backup-1', '/path/to/file1.ts'),
        createMockBackup('backup-2', '/path/to/file2.ts'),
      ]
      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          filePath="/path/to/file1.ts"
        />
      )

      // Component filters by filePath prop internally
      expect(screen.getByText('file1.ts')).toBeInTheDocument()
      expect(screen.queryByText('file2.ts')).not.toBeInTheDocument()
    })
  })

  describe('rollback workflow', () => {
    it('opens rollback modal when restore is clicked', async () => {
      const user = userEvent.setup()
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      const mockInitiateRollback = vi.fn()

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )
      ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultRollbackHook(),
        initiateRollback: mockInitiateRollback,
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onRestoreComplete={mockOnRestoreComplete}
        />
      )

      // Click restore button
      await user.click(screen.getByText('Restore'))

      // initiateRollback should be called
      expect(mockInitiateRollback).toHaveBeenCalledWith(backups[0])
    })

    it('shows modal when selectedBackup and isModalOpen are set', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )
      ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultRollbackHook(),
        selectedBackup: backups[0],
        isModalOpen: true,
        state: { status: 'confirming' as const },
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onRestoreComplete={mockOnRestoreComplete}
        />
      )

      // Modal should be visible
      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('Rollback File')).toBeInTheDocument()
    })

    it('completes restore when confirmed', async () => {
      const user = userEvent.setup()
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      const mockConfirmRollback = vi.fn().mockResolvedValue({ success: true })
      const mockUpdateBackup = vi.fn()

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultBackupHistoryHook(backups),
        updateBackup: mockUpdateBackup,
      })
      ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultRollbackHook(),
        selectedBackup: backups[0],
        isModalOpen: true,
        state: { status: 'confirming' as const },
        confirmRollback: mockConfirmRollback,
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onRestoreComplete={mockOnRestoreComplete}
        />
      )

      // Confirm restore
      await user.click(screen.getByText('Restore Backup'))

      // confirmRollback should be called
      expect(mockConfirmRollback).toHaveBeenCalled()
    })

    it('closes modal when cancelled', async () => {
      const user = userEvent.setup()
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      const mockCancelRollback = vi.fn()

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )
      ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultRollbackHook(),
        selectedBackup: backups[0],
        isModalOpen: true,
        state: { status: 'confirming' as const },
        cancelRollback: mockCancelRollback,
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onRestoreComplete={mockOnRestoreComplete}
        />
      )

      // Cancel
      await user.click(screen.getByText('Cancel'))

      // cancelRollback should be called
      expect(mockCancelRollback).toHaveBeenCalled()
    })
  })

  describe('delete workflow', () => {
    it('deletes backup and calls callback', async () => {
      const user = userEvent.setup()
      const backups = [
        createMockBackup('backup-1', '/path/to/file1.ts'),
        createMockBackup('backup-2', '/path/to/file2.ts'),
      ]
      const mockDeleteBackup = vi.fn().mockResolvedValue(true)

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultBackupHistoryHook(backups),
        deleteBackup: mockDeleteBackup,
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onDeleteComplete={mockOnDeleteComplete}
        />
      )

      // Click delete button on first backup
      const deleteButtons = screen.getAllByText('Delete')
      await user.click(deleteButtons[0])

      expect(mockDeleteBackup).toHaveBeenCalledWith('backup-1')
      await waitFor(() => {
        expect(mockOnDeleteComplete).toHaveBeenCalledWith('backup-1')
      })
    })
  })

  describe('error handling', () => {
    it('displays combined error from hooks', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultBackupHistoryHook(backups),
        error: 'Failed to load backups',
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onError={mockOnError}
        />
      )

      expect(screen.getByText('Failed to load backups')).toBeInTheDocument()
    })

    it('displays rollback error', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )
      ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultRollbackHook(),
        error: 'Failed to fetch diff',
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          onError={mockOnError}
        />
      )

      expect(screen.getByText('Failed to fetch diff')).toBeInTheDocument()
    })
  })

  describe('compact mode', () => {
    it('renders in compact mode when compact prop is true', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          compact
        />
      )

      expect(screen.getByText('file.ts')).toBeInTheDocument()

      // In compact mode, button text is hidden
      expect(screen.queryByText('Restore')).not.toBeInTheDocument()
      // But the button should exist with aria-label
      expect(screen.getByLabelText('Restore this backup')).toBeInTheDocument()
    })
  })

  describe('loading state', () => {
    it('shows loading state during initial fetch', () => {
      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultBackupHistoryHook([]),
        isInitialLoading: true,
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
        />
      )

      // Should show loading
      expect(screen.getByText('Loading backups...')).toBeInTheDocument()
    })

    it('disables panel while rollback is loading', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]

      ;(useBackupHistory as ReturnType<typeof vi.fn>).mockReturnValue(
        createDefaultBackupHistoryHook(backups)
      )
      ;(useRollback as ReturnType<typeof vi.fn>).mockReturnValue({
        ...createDefaultRollbackHook(),
        isLoading: true,
      })

      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
        />
      )

      // Search input should be disabled when rollback is loading
      expect(screen.getByPlaceholderText('Search backups...')).toBeDisabled()
    })
  })

  describe('hook initialization', () => {
    it('initializes useBackupHistory with correct config', () => {
      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          sessionId="session-123"
          autoRefresh
          refreshInterval={5000}
        />
      )

      expect(useBackupHistory).toHaveBeenCalledWith({
        baseUrl: 'http://localhost:8000',
        sessionId: 'session-123',
        autoRefresh: true,
        refreshInterval: 5000,
      })
    })

    it('initializes useRollback with correct config', () => {
      render(
        <RollbackIntegration
          apiBaseUrl="http://localhost:8000"
          sessionId="session-123"
        />
      )

      expect(useRollback).toHaveBeenCalledWith({
        baseUrl: 'http://localhost:8000',
        sessionId: 'session-123',
      })
    })
  })
})
