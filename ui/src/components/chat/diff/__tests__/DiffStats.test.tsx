/**
 * Tests for DiffStats Component
 */
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import DiffStats from '../DiffStats'
import type { PatchStats } from '../../../../types/diff'

describe('DiffStats', () => {
  const sampleStats: PatchStats = {
    filesChanged: 3,
    additions: 25,
    deletions: 10,
    hunks: 5,
  }

  describe('compact mode', () => {
    it('renders file count correctly', () => {
      render(<DiffStats stats={sampleStats} compact />)

      expect(screen.getByText('3 files')).toBeInTheDocument()
    })

    it('renders singular file count', () => {
      const singleFileStats = { ...sampleStats, filesChanged: 1 }
      render(<DiffStats stats={singleFileStats} compact />)

      expect(screen.getByText('1 file')).toBeInTheDocument()
    })

    it('renders additions with plus sign', () => {
      render(<DiffStats stats={sampleStats} compact />)

      expect(screen.getByText('+25')).toBeInTheDocument()
    })

    it('renders deletions with minus sign', () => {
      render(<DiffStats stats={sampleStats} compact />)

      expect(screen.getByText('-10')).toBeInTheDocument()
    })
  })

  describe('full mode', () => {
    it('renders all stats in full mode', () => {
      render(<DiffStats stats={sampleStats} />)

      expect(screen.getByText('Files changed:')).toBeInTheDocument()
      expect(screen.getByText('3')).toBeInTheDocument()
      expect(screen.getByText('Additions:')).toBeInTheDocument()
      expect(screen.getByText('+25')).toBeInTheDocument()
      expect(screen.getByText('Deletions:')).toBeInTheDocument()
      expect(screen.getByText('-10')).toBeInTheDocument()
      expect(screen.getByText('Hunks:')).toBeInTheDocument()
      expect(screen.getByText('5')).toBeInTheDocument()
    })

    it('hides hunk count when zero', () => {
      const noHunksStats = { ...sampleStats, hunks: 0 }
      render(<DiffStats stats={noHunksStats} />)

      expect(screen.queryByText('Hunks:')).not.toBeInTheDocument()
    })

    it('renders the diff bar visualization', () => {
      render(<DiffStats stats={sampleStats} />)

      // The bar should be present (checking by title attributes)
      expect(screen.getByTitle('25 additions')).toBeInTheDocument()
      expect(screen.getByTitle('10 deletions')).toBeInTheDocument()
    })

    it('does not render diff bar when no changes', () => {
      const noChangesStats = { ...sampleStats, additions: 0, deletions: 0 }
      render(<DiffStats stats={noChangesStats} />)

      expect(screen.queryByTitle(/additions/)).not.toBeInTheDocument()
      expect(screen.queryByTitle(/deletions/)).not.toBeInTheDocument()
    })
  })

  describe('edge cases', () => {
    it('handles zero values gracefully', () => {
      const zeroStats: PatchStats = {
        filesChanged: 0,
        additions: 0,
        deletions: 0,
        hunks: 0,
      }

      render(<DiffStats stats={zeroStats} compact />)

      expect(screen.getByText('0 files')).toBeInTheDocument()
      expect(screen.getByText('+0')).toBeInTheDocument()
      expect(screen.getByText('-0')).toBeInTheDocument()
    })

    it('handles large numbers', () => {
      const largeStats: PatchStats = {
        filesChanged: 1000,
        additions: 50000,
        deletions: 30000,
        hunks: 500,
      }

      render(<DiffStats stats={largeStats} compact />)

      expect(screen.getByText('1000 files')).toBeInTheDocument()
      expect(screen.getByText('+50000')).toBeInTheDocument()
      expect(screen.getByText('-30000')).toBeInTheDocument()
    })

    it('handles additions-only changes', () => {
      const addOnlyStats: PatchStats = {
        filesChanged: 1,
        additions: 100,
        deletions: 0,
        hunks: 1,
      }

      render(<DiffStats stats={addOnlyStats} />)

      // Only the additions segment should have a title
      expect(screen.getByTitle('100 additions')).toBeInTheDocument()
      expect(screen.queryByTitle('0 deletions')).not.toBeInTheDocument()
    })

    it('handles deletions-only changes', () => {
      const delOnlyStats: PatchStats = {
        filesChanged: 1,
        additions: 0,
        deletions: 50,
        hunks: 1,
      }

      render(<DiffStats stats={delOnlyStats} />)

      // Only the deletions segment should have a title
      expect(screen.getByTitle('50 deletions')).toBeInTheDocument()
      expect(screen.queryByTitle('0 additions')).not.toBeInTheDocument()
    })
  })
})
