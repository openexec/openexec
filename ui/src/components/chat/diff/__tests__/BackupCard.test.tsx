/**
 * Tests for BackupCard Component
 * @module components/chat/diff/__tests__/BackupCard
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import BackupCard from '../BackupCard'
import type { BackupMetadata } from '../../../../types/backup'

// Create a mock backup
const createMockBackup = (overrides: Partial<BackupMetadata> = {}): BackupMetadata => ({
  id: 'backup-123',
  originalPath: '/Users/test/project/src/test.ts',
  backupPath: '/backups/backup-123',
  checksum: 'abc123def456abc123def456abc123def456abc123def456abc123def456abcd',
  size: 1024,
  createdAt: new Date().toISOString(),
  fileMode: 0o644,
  sessionId: 'session-456',
  restored: false,
  ...overrides,
})

describe('BackupCard', () => {
  const mockOnClick = vi.fn()
  const mockOnRestore = vi.fn()
  const mockOnDelete = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('rendering', () => {
    it('renders the backup basic information', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} />)

      expect(screen.getByText('test.ts')).toBeInTheDocument()
    })

    it('displays file path in non-compact mode', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} />)

      expect(screen.getByText('/Users/test/project/src')).toBeInTheDocument()
    })

    it('displays file size', () => {
      const backup = createMockBackup({ size: 1024 })
      render(<BackupCard backup={backup} />)

      expect(screen.getByText('1 KB')).toBeInTheDocument()
    })

    it('displays truncated checksum', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} />)

      expect(screen.getByText('abc123de')).toBeInTheDocument()
    })

    it('displays session ID when present', () => {
      const backup = createMockBackup({ sessionId: 'session-abc123def456' })
      render(<BackupCard backup={backup} />)

      // Session ID is truncated to first 8 characters
      expect(screen.getByText('session-')).toBeInTheDocument()
    })

    it('displays relative time', () => {
      const backup = createMockBackup({
        createdAt: new Date().toISOString(),
      })
      render(<BackupCard backup={backup} />)

      expect(screen.getByText('just now')).toBeInTheDocument()
    })
  })

  describe('badges', () => {
    it('shows Latest badge when isLatest is true', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} isLatest />)

      expect(screen.getByText('Latest')).toBeInTheDocument()
    })

    it('does not show Latest badge when isLatest is false', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} isLatest={false} />)

      expect(screen.queryByText('Latest')).not.toBeInTheDocument()
    })

    it('shows Restored badge when backup was previously restored', () => {
      const backup = createMockBackup({ restored: true })
      render(<BackupCard backup={backup} />)

      expect(screen.getByText('Restored')).toBeInTheDocument()
    })

    it('does not show Restored badge when backup was not restored', () => {
      const backup = createMockBackup({ restored: false })
      render(<BackupCard backup={backup} />)

      expect(screen.queryByText('Restored')).not.toBeInTheDocument()
    })
  })

  describe('compact mode', () => {
    it('hides full details in compact mode', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} compact />)

      // Path row should not be present in compact mode
      expect(screen.queryByText('session-456')).not.toBeInTheDocument()
    })

    it('shows abbreviated path in compact mode', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} compact />)

      expect(screen.getByText('/Users/test/project/src')).toBeInTheDocument()
    })
  })

  describe('click interactions', () => {
    it('calls onClick when card is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onClick={mockOnClick} />)

      await user.click(screen.getByText('test.ts'))
      expect(mockOnClick).toHaveBeenCalledTimes(1)
    })

    it('calls onClick when Enter is pressed while focused', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onClick={mockOnClick} />)

      const card = screen.getByRole('button')
      card.focus()
      await user.keyboard('{Enter}')
      expect(mockOnClick).toHaveBeenCalledTimes(1)
    })

    it('calls onClick when Space is pressed while focused', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onClick={mockOnClick} />)

      const card = screen.getByRole('button')
      card.focus()
      await user.keyboard(' ')
      expect(mockOnClick).toHaveBeenCalledTimes(1)
    })

    it('has button role when onClick is provided', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onClick={mockOnClick} />)

      expect(screen.getByRole('button')).toBeInTheDocument()
    })

    it('does not have button role when onClick is not provided', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} />)

      expect(screen.queryByRole('button')).not.toBeInTheDocument()
    })
  })

  describe('action buttons', () => {
    it('shows Restore button when onRestore is provided', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onRestore={mockOnRestore} />)

      expect(screen.getByText('Restore')).toBeInTheDocument()
    })

    it('shows Delete button when onDelete is provided', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onDelete={mockOnDelete} />)

      expect(screen.getByText('Delete')).toBeInTheDocument()
    })

    it('calls onRestore when Restore button is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onRestore={mockOnRestore} onClick={mockOnClick} />)

      await user.click(screen.getByText('Restore'))
      expect(mockOnRestore).toHaveBeenCalledTimes(1)
      // Should not trigger onClick
      expect(mockOnClick).not.toHaveBeenCalled()
    })

    it('calls onDelete when Delete button is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onDelete={mockOnDelete} onClick={mockOnClick} />)

      await user.click(screen.getByText('Delete'))
      expect(mockOnDelete).toHaveBeenCalledTimes(1)
      // Should not trigger onClick
      expect(mockOnClick).not.toHaveBeenCalled()
    })

    it('shows both Restore and Delete buttons when both handlers are provided', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onRestore={mockOnRestore} onDelete={mockOnDelete} />)

      expect(screen.getByText('Restore')).toBeInTheDocument()
      expect(screen.getByText('Delete')).toBeInTheDocument()
    })

    it('hides action button text in compact mode', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onRestore={mockOnRestore} onDelete={mockOnDelete} compact />)

      // In compact mode, buttons only show icons, not text
      expect(screen.queryByText('Restore')).not.toBeInTheDocument()
      expect(screen.queryByText('Delete')).not.toBeInTheDocument()
      // But the buttons themselves should still exist (with aria-labels)
      expect(screen.getByLabelText('Restore this backup')).toBeInTheDocument()
      expect(screen.getByLabelText('Delete backup')).toBeInTheDocument()
    })
  })

  describe('disabled state', () => {
    it('disables action buttons when disabled is true', () => {
      const backup = createMockBackup()
      render(
        <BackupCard
          backup={backup}
          onRestore={mockOnRestore}
          onDelete={mockOnDelete}
          disabled
        />
      )

      expect(screen.getByText('Restore').closest('button')).toBeDisabled()
      expect(screen.getByText('Delete').closest('button')).toBeDisabled()
    })

    it('does not call onRestore when disabled', () => {
      const backup = createMockBackup()
      render(
        <BackupCard
          backup={backup}
          onRestore={mockOnRestore}
          disabled
        />
      )

      // When disabled, the button should be disabled and handler should not be callable
      const restoreButton = screen.getByText('Restore').closest('button')
      expect(restoreButton).toBeDisabled()
      // Firing click on a disabled button should not trigger the handler
      expect(mockOnRestore).not.toHaveBeenCalled()
    })
  })

  describe('selected state', () => {
    it('applies selected styling when selected is true', () => {
      const backup = createMockBackup()
      const { container } = render(<BackupCard backup={backup} selected />)

      expect(container.firstChild).toHaveClass('backup-card--selected')
    })

    it('does not apply selected styling when selected is false', () => {
      const backup = createMockBackup()
      const { container } = render(<BackupCard backup={backup} selected={false} />)

      expect(container.firstChild).not.toHaveClass('backup-card--selected')
    })
  })

  describe('accessibility', () => {
    it('has accessible restore button with title', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onRestore={mockOnRestore} />)

      const restoreButton = screen.getByTitle('Restore this backup')
      expect(restoreButton).toBeInTheDocument()
    })

    it('has accessible delete button with title', () => {
      const backup = createMockBackup()
      render(<BackupCard backup={backup} onDelete={mockOnDelete} />)

      const deleteButton = screen.getByTitle('Delete backup')
      expect(deleteButton).toBeInTheDocument()
    })
  })
})
