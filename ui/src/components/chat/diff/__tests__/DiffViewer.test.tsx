/**
 * Tests for DiffViewer Component
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import DiffViewer from '../DiffViewer'
import type { Patch, PatchStats } from '../../../../types/diff'

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

const createBinaryFilePatch = (): Patch => ({
  files: [
    {
      oldName: 'image.png',
      newName: 'image.png',
      hunks: [],
      isBinary: true,
      isNew: false,
      isDeleted: false,
      isRenamed: false,
      gitHeaders: [],
    },
  ],
  rawContent: 'Binary files differ',
})

const createRenamedFilePatch = (): Patch => ({
  files: [
    {
      oldName: 'old-name.ts',
      newName: 'new-name.ts',
      hunks: [],
      isBinary: false,
      isNew: false,
      isDeleted: false,
      isRenamed: true,
      gitHeaders: ['rename from old-name.ts', 'rename to new-name.ts'],
    },
  ],
  rawContent: '',
})

const createDeletedFilePatch = (): Patch => ({
  files: [
    {
      oldName: 'deleted.ts',
      newName: '/dev/null',
      hunks: [
        {
          oldStart: 1,
          oldCount: 2,
          newStart: 0,
          newCount: 0,
          header: '@@ -1,2 +0,0 @@',
          lines: [
            { type: 'remove', content: 'const deleted = true' },
            { type: 'remove', content: 'export { deleted }' },
          ],
        },
      ],
      isBinary: false,
      isNew: false,
      isDeleted: true,
      isRenamed: false,
      gitHeaders: [],
    },
  ],
  rawContent: '',
})

describe('DiffViewer', () => {
  describe('rendering', () => {
    it('renders with a simple patch', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('Changes')).toBeInTheDocument()
      expect(screen.getByText('src/test.ts')).toBeInTheDocument()
    })

    it('renders multiple files', () => {
      const patch = createMultiFilePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('src/file1.ts')).toBeInTheDocument()
      expect(screen.getByText('src/newfile.ts')).toBeInTheDocument()
    })

    it('shows stats in header', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} showStats />)

      expect(screen.getByText('1 file')).toBeInTheDocument()
      // Stats show up in both the header (DiffStats) and file card - check at least one exists
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

      render(<DiffViewer patch={patch} stats={customStats} showStats />)

      // The header stats should use the custom stats (5 files, +100, -50)
      expect(screen.getByText('5 files')).toBeInTheDocument()
      // Header shows custom stats, file card shows actual file stats
      expect(screen.getByText('+100')).toBeInTheDocument()
      expect(screen.getByText('-50')).toBeInTheDocument()
    })

    it('shows raw content fallback for empty patches', () => {
      const patch: Patch = {
        files: [],
        rawContent: 'Raw patch content here',
      }

      render(<DiffViewer patch={patch} showRawFallback />)

      expect(screen.getByText('Raw patch content here')).toBeInTheDocument()
    })

    it('hides raw fallback when disabled', () => {
      const patch: Patch = {
        files: [],
        rawContent: 'Raw patch content here',
      }

      render(<DiffViewer patch={patch} showRawFallback={false} />)

      expect(screen.queryByText('Raw patch content here')).not.toBeInTheDocument()
    })
  })

  describe('file badges', () => {
    it('shows "New" badge for new files', () => {
      const patch = createMultiFilePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('New')).toBeInTheDocument()
    })

    it('shows "Deleted" badge for deleted files', () => {
      const patch = createDeletedFilePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('Deleted')).toBeInTheDocument()
    })

    it('shows "Renamed" badge for renamed files', () => {
      const patch = createRenamedFilePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('Renamed')).toBeInTheDocument()
    })

    it('shows "Binary" badge for binary files', () => {
      const patch = createBinaryFilePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('Binary')).toBeInTheDocument()
    })
  })

  describe('expand/collapse functionality', () => {
    it('files are expanded by default', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} defaultExpanded />)

      // Hunk header should be visible when expanded
      expect(screen.getByText(/@@ -1,3 \+1,4 @@/)).toBeInTheDocument()
    })

    it('files are collapsed when defaultExpanded is false', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} defaultExpanded={false} />)

      // Hunk header should not be visible when collapsed
      expect(screen.queryByText(/@@ -1,3 \+1,4 @@/)).not.toBeInTheDocument()
    })

    it('toggles file expansion on click', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} defaultExpanded />)

      // Initially expanded
      expect(screen.getByText(/@@ -1,3 \+1,4 @@/)).toBeInTheDocument()

      // Click file header to collapse
      fireEvent.click(screen.getByText('src/test.ts'))

      // Should be collapsed now
      expect(screen.queryByText(/@@ -1,3 \+1,4 @@/)).not.toBeInTheDocument()

      // Click again to expand
      fireEvent.click(screen.getByText('src/test.ts'))

      // Should be expanded again
      expect(screen.getByText(/@@ -1,3 \+1,4 @@/)).toBeInTheDocument()
    })

    it('expand all button expands all files', () => {
      const patch = createMultiFilePatch()
      render(<DiffViewer patch={patch} defaultExpanded={false} />)

      // Click expand all
      fireEvent.click(screen.getByText('Expand all'))

      // Both files should show their hunks
      expect(screen.getByText('@@ -10,2 +10,3 @@')).toBeInTheDocument()
      expect(screen.getByText('@@ -0,0 +1,2 @@')).toBeInTheDocument()
    })

    it('collapse all button collapses all files', () => {
      const patch = createMultiFilePatch()
      render(<DiffViewer patch={patch} defaultExpanded />)

      // Click collapse all
      fireEvent.click(screen.getByText('Collapse all'))

      // Hunks should not be visible
      expect(screen.queryByText('@@ -10,2 +10,3 @@')).not.toBeInTheDocument()
      expect(screen.queryByText('@@ -0,0 +1,2 @@')).not.toBeInTheDocument()
    })
  })

  describe('line selection', () => {
    it('calls onLineSelect when a line is clicked', () => {
      const patch = createSimplePatch()
      const onLineSelect = vi.fn()

      render(<DiffViewer patch={patch} onLineSelect={onLineSelect} />)

      // Find and click the added line
      const addedLineContent = screen.getByText('const bar = 3')
      fireEvent.click(addedLineContent.closest('tr')!)

      expect(onLineSelect).toHaveBeenCalledWith(
        expect.objectContaining({
          fileIndex: 0,
          hunkIndex: 0,
          line: expect.objectContaining({
            type: 'add',
            content: 'const bar = 3',
          }),
        })
      )
    })

    it('provides correct line numbers in selection callback', () => {
      const patch = createSimplePatch()
      const onLineSelect = vi.fn()

      render(<DiffViewer patch={patch} onLineSelect={onLineSelect} />)

      // Click a context line
      const contextLine = screen.getByText('const foo = 1')
      fireEvent.click(contextLine.closest('tr')!)

      expect(onLineSelect).toHaveBeenCalledWith(
        expect.objectContaining({
          oldLineNumber: 1,
          newLineNumber: 1,
        })
      )

      // Click an added line
      const addedLine = screen.getByText('const bar = 3')
      fireEvent.click(addedLine.closest('tr')!)

      expect(onLineSelect).toHaveBeenCalledWith(
        expect.objectContaining({
          oldLineNumber: null,
          newLineNumber: 2,
        })
      )
    })
  })

  describe('binary files', () => {
    it('shows binary file message', () => {
      const patch = createBinaryFilePatch()
      render(<DiffViewer patch={patch} />)

      expect(screen.getByText('Binary file not shown')).toBeInTheDocument()
    })
  })

  describe('keyboard navigation', () => {
    it('toggles file on Enter key', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} defaultExpanded />)

      const fileHeader = screen.getByText('src/test.ts').closest('[role="button"]')!

      // Initially expanded
      expect(screen.getByText(/@@ -1,3 \+1,4 @@/)).toBeInTheDocument()

      // Press Enter to collapse
      fireEvent.keyDown(fileHeader, { key: 'Enter' })
      expect(screen.queryByText(/@@ -1,3 \+1,4 @@/)).not.toBeInTheDocument()

      // Press Enter to expand
      fireEvent.keyDown(fileHeader, { key: 'Enter' })
      expect(screen.getByText(/@@ -1,3 \+1,4 @@/)).toBeInTheDocument()
    })

    it('toggles file on Space key', () => {
      const patch = createSimplePatch()
      render(<DiffViewer patch={patch} defaultExpanded />)

      const fileHeader = screen.getByText('src/test.ts').closest('[role="button"]')!

      // Press Space to collapse
      fireEvent.keyDown(fileHeader, { key: ' ' })
      expect(screen.queryByText(/@@ -1,3 \+1,4 @@/)).not.toBeInTheDocument()
    })
  })
})
