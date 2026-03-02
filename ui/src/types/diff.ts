/**
 * Diff/Patch Types for OpenExec UI
 *
 * These types mirror the Go backend types from:
 * - internal/mcp/patch.go (Patch, PatchFile, PatchHunk, PatchLine)
 *
 * @module types/diff
 */

// =============================================================================
// Patch Line Types
// =============================================================================

/**
 * Type of a patch line
 */
export type LineType = 'context' | 'add' | 'remove'

/**
 * Single line in a patch hunk
 */
export interface PatchLine {
  /** Type of line (context, add, remove) */
  type: LineType
  /** Line content (without the prefix character) */
  content: string
}

// =============================================================================
// Patch Hunk Types
// =============================================================================

/**
 * Single hunk in a unified diff
 */
export interface PatchHunk {
  /** Starting line in old file */
  oldStart: number
  /** Number of lines in old file */
  oldCount: number
  /** Starting line in new file */
  newStart: number
  /** Number of lines in new file */
  newCount: number
  /** The full @@ header line */
  header: string
  /** Lines in this hunk */
  lines: PatchLine[]
}

// =============================================================================
// Patch File Types
// =============================================================================

/**
 * Single file modification in a unified diff patch
 */
export interface PatchFile {
  /** Original file path (from --- line) */
  oldName: string
  /** New file path (from +++ line) */
  newName: string
  /** List of hunks in this file */
  hunks: PatchHunk[]
  /** True if this is a binary file */
  isBinary: boolean
  /** True if this is a new file */
  isNew: boolean
  /** True if this is a deleted file */
  isDeleted: boolean
  /** True if this is a rename operation */
  isRenamed: boolean
  /** Git-specific headers (index, mode, etc.) */
  gitHeaders: string[]
}

// =============================================================================
// Patch Types
// =============================================================================

/**
 * Complete unified diff patch
 */
export interface Patch {
  /** Files modified by this patch */
  files: PatchFile[]
  /** Original patch content */
  rawContent: string
}

/**
 * Statistics about a patch
 */
export interface PatchStats {
  /** Number of files changed */
  filesChanged: number
  /** Number of lines added */
  additions: number
  /** Number of lines deleted */
  deletions: number
  /** Total number of hunks */
  hunks: number
}

// =============================================================================
// UI State Types
// =============================================================================

/**
 * Line selection info for interactive features
 */
export interface LineSelectInfo {
  /** File index in patch */
  fileIndex: number
  /** Hunk index in file */
  hunkIndex: number
  /** Line index in hunk */
  lineIndex: number
  /** The selected line */
  line: PatchLine
  /** Old line number (null for additions) */
  oldLineNumber: number | null
  /** New line number (null for deletions) */
  newLineNumber: number | null
}

/**
 * Expansion state for files
 */
export interface FileExpansionState {
  [fileIndex: number]: boolean
}

/**
 * Expansion state for hunks within a file
 */
export interface HunkExpansionState {
  [fileIndex: number]: {
    [hunkIndex: number]: boolean
  }
}
