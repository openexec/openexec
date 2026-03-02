/**
 * Tests for RollbackModal Component
 * @module components/chat/diff/__tests__/RollbackModal
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import RollbackModal from '../RollbackModal'
import type { BackupMetadata } from '../../../../types/backup'
import type { Patch } from '../../../../types/diff'

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

// Create a simple patch for diff preview
const createMockDiff = (): Patch => ({
  files: [
    {
      oldName: 'src/test.ts',
      newName: 'src/test.ts',
      hunks: [
        {
          oldStart: 1,
          oldCount: 3,
          newStart: 1,
          newCount: 2,
          header: '@@ -1,3 +1,2 @@',
          lines: [
            { type: 'context', content: 'const foo = 1' },
            { type: 'remove', content: 'const bar = 2' },
            { type: 'context', content: 'export { foo }' },
          ],
        },
      ],
      isBinary: false,
      isNew: false,
      isDeleted: false,
      isRenamed: false,
      gitHeaders: [],
    },
  ],
  rawContent: '',
})

describe('RollbackModal', () => {
  const mockOnClose = vi.fn()
  const mockOnConfirm = vi.fn()
  const mockOnCancel = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('rendering', () => {
    it('renders nothing when isOpen is false', () => {
      const backup = createMockBackup()
      const { container } = render(
        <RollbackModal
          backup={backup}
          isOpen={false}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      expect(container.firstChild).toBeNull()
    })

    it('renders dialog when isOpen is true', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('Rollback File')).toBeInTheDocument()
    })

    it('renders custom title when provided', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          title="Restore to Previous Version"
        />
      )
      expect(screen.getByText('Restore to Previous Version')).toBeInTheDocument()
    })

    it('displays file name and directory', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      expect(screen.getByText('test.ts')).toBeInTheDocument()
      expect(screen.getByText('/Users/test/project/src')).toBeInTheDocument()
    })

    it('displays backup size', () => {
      const backup = createMockBackup({ size: 1024 })
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      expect(screen.getByText('1 KB')).toBeInTheDocument()
    })

    it('displays truncated checksum', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      expect(screen.getByText('abc123def456abc1...')).toBeInTheDocument()
    })

    it('displays session ID when present', () => {
      const backup = createMockBackup({ sessionId: 'session-abc123' })
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      // Session ID shows first 8 chars + "..."
      expect(screen.getByText('session-...')).toBeInTheDocument()
    })

    it('shows previously restored badge when backup was restored before', () => {
      const backup = createMockBackup({ restored: true })
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )
      expect(screen.getByText('Previously Restored')).toBeInTheDocument()
    })
  })

  describe('warnings and errors', () => {
    it('renders warning messages', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          warnings={['File has been modified', 'Backup is older than 7 days']}
        />
      )
      expect(screen.getByText('File has been modified')).toBeInTheDocument()
      expect(screen.getByText('Backup is older than 7 days')).toBeInTheDocument()
    })

    it('shows info message when current file does not exist', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          currentFileExists={false}
        />
      )
      expect(
        screen.getByText('The original file no longer exists. Restoring will recreate it.')
      ).toBeInTheDocument()
    })

    it('displays error message when provided', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="error"
          error="Backup file is corrupted"
        />
      )
      expect(screen.getByText('Backup file is corrupted')).toBeInTheDocument()
    })

    it('displays success message when restore is complete', () => {
      const backup = createMockBackup()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="success"
        />
      )
      expect(screen.getByText('File successfully restored to backup version!')).toBeInTheDocument()
    })
  })

  describe('diff preview', () => {
    it('renders diff viewer when diff is provided', () => {
      const backup = createMockBackup()
      const diff = createMockDiff()
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          diff={diff}
        />
      )
      expect(screen.getByText('Changes to be reverted')).toBeInTheDocument()
      expect(screen.getByText('src/test.ts')).toBeInTheDocument()
    })

    it('does not render diff section when diff is empty', () => {
      const backup = createMockBackup()
      const emptyDiff: Patch = { files: [], rawContent: '' }
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          diff={emptyDiff}
        />
      )
      expect(screen.queryByText('Changes to be reverted')).not.toBeInTheDocument()
    })
  })

  describe('actions', () => {
    it('calls onConfirm when Restore Backup button is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      await user.click(screen.getByText('Restore Backup'))
      expect(mockOnConfirm).toHaveBeenCalledTimes(1)
    })

    it('calls onClose when Cancel button is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      await user.click(screen.getByText('Cancel'))
      expect(mockOnClose).toHaveBeenCalledTimes(1)
    })

    it('calls onCancel and onClose when Cancel button is clicked with onCancel', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          onCancel={mockOnCancel}
        />
      )

      await user.click(screen.getByText('Cancel'))
      expect(mockOnCancel).toHaveBeenCalledTimes(1)
      expect(mockOnClose).toHaveBeenCalledTimes(1)
    })

    it('calls onClose when close button is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      await user.click(screen.getByLabelText('Close dialog'))
      expect(mockOnClose).toHaveBeenCalledTimes(1)
    })

    it('shows Close button instead of Cancel when status is success', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="success"
        />
      )

      expect(screen.getByText('Close')).toBeInTheDocument()
      expect(screen.queryByText('Cancel')).not.toBeInTheDocument()
    })

    it('hides Restore Backup button when status is success', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="success"
        />
      )

      expect(screen.queryByText('Restore Backup')).not.toBeInTheDocument()
    })
  })

  describe('backdrop and keyboard interactions', () => {
    it('calls onClose when backdrop is clicked', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      const backdrop = screen.getByRole('dialog')
      await user.click(backdrop)
      expect(mockOnClose).toHaveBeenCalled()
    })

    it('handles escape key to close dialog', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
      expect(mockOnClose).toHaveBeenCalled()
    })

    it('does not close on escape when loading', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="restoring"
        />
      )

      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
      expect(mockOnClose).not.toHaveBeenCalled()
    })

    it('does not close on backdrop click when loading', async () => {
      const user = userEvent.setup()
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="restoring"
        />
      )

      const backdrop = screen.getByRole('dialog')
      await user.click(backdrop)
      expect(mockOnClose).not.toHaveBeenCalled()
    })
  })

  describe('loading state', () => {
    it('disables Restore button when loading', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="restoring"
        />
      )

      // Find the button with the loading text (there may be multiple "Restoring..." texts)
      const buttons = screen.getAllByRole('button')
      const restoreButton = buttons.find(btn => btn.className.includes('confirm'))
      expect(restoreButton).toBeDisabled()
    })

    it('shows loading text on Restore button when restoring', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="restoring"
        />
      )

      // There are multiple "Restoring..." texts (status indicator and button)
      const restoringTexts = screen.getAllByText('Restoring...')
      expect(restoringTexts.length).toBeGreaterThan(0)
    })

    it('disables Cancel button when loading', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="restoring"
        />
      )

      expect(screen.getByText('Cancel').closest('button')).toBeDisabled()
    })

    it('disables close button when loading', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="restoring"
        />
      )

      expect(screen.getByLabelText('Close dialog')).toBeDisabled()
    })

    it('displays status label in footer', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="confirming"
        />
      )

      expect(screen.getByText('Confirm Rollback')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('disables Restore button when there is an error', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          status="error"
          error="Something went wrong"
        />
      )

      const restoreButton = screen.getByText('Restore Backup').closest('button')
      expect(restoreButton).toBeDisabled()
    })
  })

  describe('file size comparison', () => {
    it('displays current file size when provided', () => {
      const backup = createMockBackup({ size: 1024 })
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          currentFileSize={2048}
        />
      )
      expect(screen.getByText('2 KB')).toBeInTheDocument()
    })

    it('shows size difference when current file is larger', () => {
      const backup = createMockBackup({ size: 1024 })
      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
          currentFileSize={2048}
        />
      )
      expect(screen.getByText('(+1 KB)')).toBeInTheDocument()
    })
  })

  describe('accessibility', () => {
    it('has correct dialog role and attributes', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      const dialog = screen.getByRole('dialog')
      expect(dialog).toHaveAttribute('aria-modal', 'true')
      expect(dialog).toHaveAttribute('aria-labelledby', 'rollback-modal-title')
    })

    it('has accessible close button', () => {
      const backup = createMockBackup()

      render(
        <RollbackModal
          backup={backup}
          isOpen={true}
          onClose={mockOnClose}
          onConfirm={mockOnConfirm}
        />
      )

      expect(screen.getByLabelText('Close dialog')).toBeInTheDocument()
    })
  })
})
