/**
 * Tests for PatchPreviewModal Component
 * @module components/chat/diff/__tests__/PatchPreviewModal
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import PatchPreviewModal from '../PatchPreviewModal'
import type { Patch, PatchStats } from '../../../../types/diff'
import type { PatchValidationWarning } from '../PatchPreviewModal'

// Sample test patches
const createSimplePatch = (): Patch => ({
  files: [
    {
      oldName: 'src/test.ts',
      newName: 'src/test.ts',
      hunks: [
        {
          oldStart: 1,
          oldCount: 3,
          newStart: 1,
          newCount: 4,
          header: '@@ -1,3 +1,4 @@ function test()',
          lines: [
            { type: 'context', content: 'const foo = 1' },
            { type: 'remove', content: 'const bar = 2' },
            { type: 'add', content: 'const bar = 3' },
            { type: 'add', content: 'const baz = 4' },
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
  rawContent: '--- a/src/test.ts\n+++ b/src/test.ts\n@@...',
})

const createMultiFilePatch = (): Patch => ({
  files: [
    {
      oldName: 'src/file1.ts',
      newName: 'src/file1.ts',
      hunks: [
        {
          oldStart: 10,
          oldCount: 2,
          newStart: 10,
          newCount: 3,
          header: '@@ -10,2 +10,3 @@',
          lines: [
            { type: 'context', content: 'line 10' },
            { type: 'add', content: 'new line' },
            { type: 'context', content: 'line 11' },
          ],
        },
      ],
      isBinary: false,
      isNew: false,
      isDeleted: false,
      isRenamed: false,
      gitHeaders: [],
    },
    {
      oldName: '/dev/null',
      newName: 'src/newfile.ts',
      hunks: [
        {
          oldStart: 0,
          oldCount: 0,
          newStart: 1,
          newCount: 2,
          header: '@@ -0,0 +1,2 @@',
          lines: [
            { type: 'add', content: 'export const foo = 1' },
            { type: 'add', content: 'export const bar = 2' },
          ],
        },
      ],
      isBinary: false,
      isNew: true,
      isDeleted: false,
      isRenamed: false,
      gitHeaders: [],
    },
  ],
  rawContent: '',
})

const createEmptyPatch = (): Patch => ({
  files: [],
  rawContent: '',
})

describe('PatchPreviewModal', () => {
  const mockOnClose = vi.fn()
  const mockOnApply = vi.fn()
  const mockOnReject = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('rendering', () => {
    it('renders nothing when isOpen is false', () => {
      const patch = createSimplePatch()
      const { container } = render(
        <PatchPreviewModal
          patch={patch}
          isOpen={false}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )
      expect(container.firstChild).toBeNull()
    })

    it('renders dialog when isOpen is true', () => {
      const patch = createSimplePatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )
      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('Preview Changes')).toBeInTheDocument()
    })

    it('renders custom title when provided', () => {
      const patch = createSimplePatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          title="Apply Patch to Repository"
        />
      )
      expect(screen.getByText('Apply Patch to Repository')).toBeInTheDocument()
    })

    it('renders description when provided', () => {
      const patch = createSimplePatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          description="Review the changes before applying"
        />
      )
      expect(screen.getByText('Review the changes before applying')).toBeInTheDocument()
    })

    it('renders target path when provided', () => {
      const patch = createSimplePatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          targetPath="/Users/test/project"
        />
      )
      expect(screen.getByText('/Users/test/project')).toBeInTheDocument()
    })

    it('renders diff viewer with patch content', () => {
      const patch = createSimplePatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )
      expect(screen.getByText('src/test.ts')).toBeInTheDocument()
    })

    it('renders empty state for empty patch', () => {
      const patch = createEmptyPatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )
      expect(screen.getByText('No changes to preview')).toBeInTheDocument()
    })
  })

  describe('statistics', () => {
    it('calculates and displays stats', () => {
      const patch = createSimplePatch()
      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )
      expect(screen.getByText('1 file')).toBeInTheDocument()
      expect(screen.getAllByText('+2').length).toBeGreaterThan(0)
      expect(screen.getAllByText('-1').length).toBeGreaterThan(0)
    })

    it('uses provided stats when available', () => {
      const patch = createSimplePatch()
      const customStats: PatchStats = {
        filesChanged: 5,
        additions: 100,
        deletions: 50,
        hunks: 10,
      }

      render(
        <PatchPreviewModal
          patch={patch}
          stats={customStats}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      expect(screen.getByText('5 files')).toBeInTheDocument()
      expect(screen.getByText('+100')).toBeInTheDocument()
      expect(screen.getByText('-50')).toBeInTheDocument()
    })
  })

  describe('warnings', () => {
    it('renders info warnings', () => {
      const patch = createSimplePatch()
      const warnings: PatchValidationWarning[] = [
        {
          type: 'missing_context',
          message: 'Some context lines may be outdated',
        },
      ]

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          warnings={warnings}
        />
      )

      expect(screen.getByText('Some context lines may be outdated')).toBeInTheDocument()
    })

    it('renders critical warnings', () => {
      const patch = createSimplePatch()
      const warnings: PatchValidationWarning[] = [
        {
          type: 'file_not_found',
          message: 'Target file src/test.ts not found',
          filePath: 'src/test.ts',
        },
      ]

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          warnings={warnings}
        />
      )

      expect(screen.getByText('Target file src/test.ts not found')).toBeInTheDocument()
      expect(screen.getByText('This patch may not apply cleanly')).toBeInTheDocument()
    })

    it('renders multiple warnings', () => {
      const patch = createSimplePatch()
      const warnings: PatchValidationWarning[] = [
        {
          type: 'line_count_mismatch',
          message: 'Line count mismatch in hunk 1',
          hunkIndex: 0,
        },
        {
          type: 'missing_context',
          message: 'Context lines differ',
        },
      ]

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          warnings={warnings}
        />
      )

      expect(screen.getByText('Line count mismatch in hunk 1')).toBeInTheDocument()
      expect(screen.getByText('Context lines differ')).toBeInTheDocument()
    })
  })

  describe('actions', () => {
    it('calls onApply when Apply Changes button is clicked', async () => {
      const user = userEvent.setup()
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      await user.click(screen.getByText('Apply Changes'))
      expect(mockOnApply).toHaveBeenCalledTimes(1)
    })

    it('calls onReject when Reject button is clicked', async () => {
      const user = userEvent.setup()
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          onReject={mockOnReject}
        />
      )

      await user.click(screen.getByText('Reject'))
      expect(mockOnReject).toHaveBeenCalledTimes(1)
    })

    it('calls onClose when Cancel button is clicked', async () => {
      const user = userEvent.setup()
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      await user.click(screen.getByText('Cancel'))
      expect(mockOnClose).toHaveBeenCalledTimes(1)
    })

    it('calls onClose when close button is clicked', async () => {
      const user = userEvent.setup()
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      await user.click(screen.getByLabelText('Close dialog'))
      expect(mockOnClose).toHaveBeenCalledTimes(1)
    })

    it('does not show Reject button when onReject is not provided', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      expect(screen.queryByText('Reject')).not.toBeInTheDocument()
    })
  })

  describe('backdrop and keyboard interactions', () => {
    it('calls onClose when backdrop is clicked', async () => {
      const user = userEvent.setup()
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      // Click the backdrop (dialog wrapper)
      const backdrop = screen.getByRole('dialog')
      await user.click(backdrop)
      expect(mockOnClose).toHaveBeenCalled()
    })

    it('handles escape key to close dialog', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
      expect(mockOnClose).toHaveBeenCalled()
    })

    it('does not close on escape when loading', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          isLoading={true}
        />
      )

      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
      expect(mockOnClose).not.toHaveBeenCalled()
    })

    it('does not close on backdrop click when loading', async () => {
      const user = userEvent.setup()
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          isLoading={true}
        />
      )

      const backdrop = screen.getByRole('dialog')
      await user.click(backdrop)
      expect(mockOnClose).not.toHaveBeenCalled()
    })
  })

  describe('loading state', () => {
    it('disables Apply button when loading', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          isLoading={true}
        />
      )

      const buttons = screen.getAllByRole('button')
      const applyButton = buttons.find((btn) => btn.className.includes('apply'))
      expect(applyButton).toBeDisabled()
    })

    it('disables Cancel button when loading', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          isLoading={true}
        />
      )

      expect(screen.getByText('Cancel').closest('button')).toBeDisabled()
    })

    it('disables Reject button when loading', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          onReject={mockOnReject}
          isLoading={true}
        />
      )

      const buttons = screen.getAllByRole('button')
      const rejectButton = buttons.find((btn) => btn.className.includes('reject'))
      expect(rejectButton).toBeDisabled()
    })

    it('disables close button when loading', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
          isLoading={true}
        />
      )

      expect(screen.getByLabelText('Close dialog')).toBeDisabled()
    })
  })

  describe('empty patch handling', () => {
    it('disables Apply button for empty patch', () => {
      const patch = createEmptyPatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      const buttons = screen.getAllByRole('button')
      const applyButton = buttons.find((btn) => btn.className.includes('apply'))
      expect(applyButton).toBeDisabled()
    })
  })

  describe('multi-file patches', () => {
    it('displays multiple files', () => {
      const patch = createMultiFilePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      expect(screen.getByText('src/file1.ts')).toBeInTheDocument()
      expect(screen.getByText('src/newfile.ts')).toBeInTheDocument()
    })

    it('shows correct stats for multi-file patch', () => {
      const patch = createMultiFilePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      expect(screen.getByText('2 files')).toBeInTheDocument()
    })
  })

  describe('accessibility', () => {
    it('has correct dialog role', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      const dialog = screen.getByRole('dialog')
      expect(dialog).toHaveAttribute('aria-modal', 'true')
      expect(dialog).toHaveAttribute('aria-labelledby', 'patch-preview-title')
    })

    it('has accessible close button', () => {
      const patch = createSimplePatch()

      render(
        <PatchPreviewModal
          patch={patch}
          isOpen={true}
          onClose={mockOnClose}
          onApply={mockOnApply}
        />
      )

      expect(screen.getByLabelText('Close dialog')).toBeInTheDocument()
    })
  })
})
