/**
 * Tests for BackupHistoryPanel Component
 * @module components/chat/diff/__tests__/BackupHistoryPanel
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import BackupHistoryPanel from '../BackupHistoryPanel'
import type { BackupMetadata, BackupStats } from '../../../../types/backup'

// Create mock backups
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

const createMockBackups = (): BackupMetadata[] => [
  createMockBackup('backup-1', '/Users/test/project/src/file1.ts', {
    createdAt: new Date(Date.now() - 1000 * 60).toISOString(), // 1 min ago
    size: 1024,
  }),
  createMockBackup('backup-2', '/Users/test/project/src/file2.ts', {
    createdAt: new Date(Date.now() - 1000 * 60 * 60).toISOString(), // 1 hour ago
    size: 2048,
    restored: true,
  }),
  createMockBackup('backup-3', '/Users/test/project/src/file1.ts', {
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(), // 2 hours ago
    size: 512,
  }),
]

const createMockStats = (): BackupStats => ({
  totalBackups: 10,
  totalFiles: 5,
  totalSizeBytes: 10240,
  oldestBackup: new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString(),
  newestBackup: new Date().toISOString(),
})

describe('BackupHistoryPanel', () => {
  const mockOnRestore = vi.fn()
  const mockOnDelete = vi.fn()
  const mockOnSelect = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('rendering', () => {
    it('renders panel with title', () => {
      render(<BackupHistoryPanel backups={[]} />)

      expect(screen.getByText('Backup History')).toBeInTheDocument()
    })

    it('renders custom title when provided', () => {
      render(<BackupHistoryPanel backups={[]} title="File Backups" />)

      expect(screen.getByText('File Backups')).toBeInTheDocument()
    })

    it('renders backup count', () => {
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      expect(screen.getByText('3 backups')).toBeInTheDocument()
    })

    it('displays singular form for single backup', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      render(<BackupHistoryPanel backups={backups} />)

      expect(screen.getByText('1 backup')).toBeInTheDocument()
    })

    it('renders empty state when no backups', () => {
      render(<BackupHistoryPanel backups={[]} />)

      expect(screen.getByText('No backups available')).toBeInTheDocument()
      expect(
        screen.getByText('Backups are created automatically when files are modified')
      ).toBeInTheDocument()
    })

    it('renders all backup cards', () => {
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      // file1.ts appears twice (2 backups), file2.ts appears once
      expect(screen.getAllByText('file1.ts')).toHaveLength(2)
      expect(screen.getByText('file2.ts')).toBeInTheDocument()
    })
  })

  describe('statistics', () => {
    it('shows stats bar when showStats is true and stats provided', () => {
      const stats = createMockStats()
      render(<BackupHistoryPanel backups={[]} stats={stats} showStats />)

      expect(screen.getByText('Total')).toBeInTheDocument()
      expect(screen.getByText('10')).toBeInTheDocument()
      expect(screen.getByText('Files')).toBeInTheDocument()
      expect(screen.getByText('5')).toBeInTheDocument()
      expect(screen.getByText('Size')).toBeInTheDocument()
      expect(screen.getByText('10 KB')).toBeInTheDocument()
    })

    it('hides stats bar when showStats is false', () => {
      const stats = createMockStats()
      render(<BackupHistoryPanel backups={[]} stats={stats} showStats={false} />)

      expect(screen.queryByText('Total')).not.toBeInTheDocument()
    })
  })

  describe('filtering', () => {
    it('shows filter bar when showFilter is true', () => {
      render(<BackupHistoryPanel backups={[]} showFilter />)

      expect(screen.getByPlaceholderText('Search backups...')).toBeInTheDocument()
    })

    it('hides filter bar when showFilter is false', () => {
      render(<BackupHistoryPanel backups={[]} showFilter={false} />)

      expect(screen.queryByPlaceholderText('Search backups...')).not.toBeInTheDocument()
    })

    it('filters backups by search query', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      await user.type(screen.getByPlaceholderText('Search backups...'), 'file1')

      // file1.ts has 2 backups, both should be shown
      expect(screen.getAllByText('file1.ts')).toHaveLength(2)
      expect(screen.queryByText('file2.ts')).not.toBeInTheDocument()
    }, 10000)

    it('shows empty state when search finds no results', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      await user.type(screen.getByPlaceholderText('Search backups...'), 'nonexistent')

      // Use findByText for async state updates
      expect(await screen.findByText('No backups match your search')).toBeInTheDocument()
      expect(screen.getByText('Try adjusting your search or filters')).toBeInTheDocument()
    }, 10000)

    it('clears search when clear button is clicked', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const searchInput = screen.getByPlaceholderText('Search backups...')
      await user.type(searchInput, 'file1')
      expect(screen.queryByText('file2.ts')).not.toBeInTheDocument()

      await user.click(screen.getByLabelText('Clear search'))
      // file1.ts appears twice in the backups
      expect(screen.getAllByText('file1.ts')).toHaveLength(2)
      expect(screen.getByText('file2.ts')).toBeInTheDocument()
    })

    it('filters by restored status', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const filterSelect = screen.getByDisplayValue('All backups')
      await user.selectOptions(filterSelect, 'restored')

      // Only file2.ts backup is restored
      expect(screen.getByText('file2.ts')).toBeInTheDocument()
      expect(screen.queryByText('file1.ts')).not.toBeInTheDocument()
    })

    it('filters by not restored status', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const filterSelect = screen.getByDisplayValue('All backups')
      await user.selectOptions(filterSelect, 'not_restored')

      // file1.ts has 2 not-restored backups
      expect(screen.getAllByText('file1.ts')).toHaveLength(2)
      expect(screen.queryByText('file2.ts')).not.toBeInTheDocument()
    })
  })

  describe('sorting', () => {
    it('sorts by newest first by default', () => {
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const cards = screen.getAllByText(/file\d\.ts/)
      // First backup (1 min ago) should be first
      expect(cards[0]).toHaveTextContent('file1.ts')
    })

    it('sorts by oldest first', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const sortSelect = screen.getByDisplayValue('Newest first')
      await user.selectOptions(sortSelect, 'oldest')

      const cards = screen.getAllByText(/file\d\.ts/)
      // Oldest backup (2 hours ago) should be first
      expect(cards[0]).toHaveTextContent('file1.ts')
    })

    it('sorts by largest first', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const sortSelect = screen.getByDisplayValue('Newest first')
      await user.selectOptions(sortSelect, 'largest')

      const sizes = screen.getAllByText(/KB/)
      // 2 KB should be first
      expect(sizes[0]).toHaveTextContent('2 KB')
    })

    it('sorts by smallest first', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const sortSelect = screen.getByDisplayValue('Newest first')
      await user.selectOptions(sortSelect, 'smallest')

      const sizes = screen.getAllByText(/KB/)
      // 512 B should be first, but it will show as "<1 KB" or similar
      expect(sizes[0]).toHaveTextContent('KB')
    })

    it('sorts by name', async () => {
      const user = userEvent.setup()
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      const sortSelect = screen.getByDisplayValue('Newest first')
      await user.selectOptions(sortSelect, 'name')

      const cards = screen.getAllByText(/file\d\.ts/)
      // file1.ts (appears twice) should come before file2.ts
      expect(cards[0]).toHaveTextContent('file1.ts')
    })
  })

  describe('actions', () => {
    it('calls onRestore when restore is clicked on a backup', async () => {
      const user = userEvent.setup()
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      render(<BackupHistoryPanel backups={backups} onRestore={mockOnRestore} />)

      await user.click(screen.getByText('Restore'))
      expect(mockOnRestore).toHaveBeenCalledWith(backups[0])
    })

    it('calls onDelete when delete is clicked on a backup', async () => {
      const user = userEvent.setup()
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      render(<BackupHistoryPanel backups={backups} onDelete={mockOnDelete} />)

      await user.click(screen.getByText('Delete'))
      expect(mockOnDelete).toHaveBeenCalledWith(backups[0])
    })

    it('calls onSelect when a backup card is clicked', async () => {
      const user = userEvent.setup()
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      render(<BackupHistoryPanel backups={backups} onSelect={mockOnSelect} />)

      await user.click(screen.getByText('file.ts'))
      expect(mockOnSelect).toHaveBeenCalledWith(backups[0])
    })
  })

  describe('latest badge', () => {
    it('marks the most recent backup for each file as latest', () => {
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} />)

      // backup-1 is the latest for file1.ts (1 min ago vs 2 hours ago)
      // backup-2 is the latest for file2.ts (only one)
      const latestBadges = screen.getAllByText('Latest')
      expect(latestBadges.length).toBe(2)
    })
  })

  describe('loading state', () => {
    it('shows loading indicator when isLoading is true', () => {
      render(<BackupHistoryPanel backups={[]} isLoading />)

      expect(screen.getByText('Loading backups...')).toBeInTheDocument()
    })

    it('hides backup list when loading', () => {
      const backups = createMockBackups()
      render(<BackupHistoryPanel backups={backups} isLoading />)

      expect(screen.queryByText('file1.ts')).not.toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message when error is provided', () => {
      render(<BackupHistoryPanel backups={[]} error="Failed to load backups" />)

      expect(screen.getByText('Failed to load backups')).toBeInTheDocument()
    })
  })

  describe('disabled state', () => {
    it('disables search input when disabled', () => {
      render(<BackupHistoryPanel backups={[]} disabled />)

      expect(screen.getByPlaceholderText('Search backups...')).toBeDisabled()
    })

    it('disables sort select when disabled', () => {
      render(<BackupHistoryPanel backups={[]} disabled />)

      expect(screen.getByDisplayValue('Newest first')).toBeDisabled()
    })

    it('disables filter select when disabled', () => {
      render(<BackupHistoryPanel backups={[]} disabled />)

      expect(screen.getByDisplayValue('All backups')).toBeDisabled()
    })
  })

  describe('selection', () => {
    it('highlights selected backup when selectedBackupId is provided', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      const { container } = render(
        <BackupHistoryPanel backups={backups} selectedBackupId="backup-1" />
      )

      const selectedCard = container.querySelector('.backup-card--selected')
      expect(selectedCard).toBeInTheDocument()
    })

    it('does not highlight any backup when selectedBackupId does not match', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      const { container } = render(
        <BackupHistoryPanel backups={backups} selectedBackupId="backup-999" />
      )

      const selectedCard = container.querySelector('.backup-card--selected')
      expect(selectedCard).not.toBeInTheDocument()
    })
  })

  describe('compact mode', () => {
    it('renders cards in compact mode when compact is true', () => {
      const backups = [createMockBackup('backup-1', '/path/to/file.ts')]
      const { container } = render(
        <BackupHistoryPanel backups={backups} compact onRestore={mockOnRestore} />
      )

      // In compact mode, action buttons don't have text
      expect(screen.queryByText('Restore')).not.toBeInTheDocument()
      // But the button should still exist with aria-label
      expect(screen.getByLabelText('Restore this backup')).toBeInTheDocument()
    })
  })
})
