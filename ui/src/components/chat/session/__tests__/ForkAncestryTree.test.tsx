/**
 * Tests for ForkAncestryTree Component
 * @module components/chat/session/__tests__/ForkAncestryTree
 */
import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ForkAncestryTree from '../ForkAncestryTree'
import type { AncestorSession } from '../ForkAncestryTree'

const createMockAncestors = (): AncestorSession[] => [
  {
    id: 'root-session',
    title: 'Original Session',
    isRoot: true,
  },
  {
    id: 'parent-session',
    title: 'First Fork',
  },
  {
    id: 'current-session',
    title: 'Current Session',
    isCurrent: true,
  },
]

describe('ForkAncestryTree', () => {
  it('renders root session indicator when no ancestors', () => {
    render(
      <ForkAncestryTree
        ancestors={[]}
        currentSessionId="root-session"
        forkDepth={0}
      />
    )
    expect(screen.getByText('Root Session')).toBeInTheDocument()
  })

  it('renders fork ancestry header with depth', () => {
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
      />
    )
    expect(screen.getByText('Fork Ancestry (Level 2)')).toBeInTheDocument()
  })

  it('renders all ancestor sessions', () => {
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
      />
    )
    expect(screen.getByText('Original Session')).toBeInTheDocument()
    expect(screen.getByText('First Fork')).toBeInTheDocument()
    expect(screen.getByText('Current Session')).toBeInTheDocument()
  })

  it('marks root session with Root label', () => {
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
      />
    )
    // Find specifically the badge/label, not the title
    const rootLabels = screen.getAllByText('Root')
    expect(rootLabels.length).toBeGreaterThan(0)
  })

  it('shows level labels for each ancestor', () => {
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
      />
    )
    expect(screen.getByText('Level 1')).toBeInTheDocument()
    expect(screen.getByText('Level 2')).toBeInTheDocument()
  })

  it('calls onNavigateToSession when non-current ancestor is clicked', () => {
    const mockNavigate = vi.fn()
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
        onNavigateToSession={mockNavigate}
      />
    )

    // Click on parent session
    fireEvent.click(screen.getByText('First Fork').closest('button')!)
    expect(mockNavigate).toHaveBeenCalledWith('parent-session')
  })

  it('does not call onNavigateToSession when current session is clicked', () => {
    const mockNavigate = vi.fn()
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
        onNavigateToSession={mockNavigate}
      />
    )

    // Current session button should be disabled
    const currentButton = screen.getByText('Current Session').closest('button')
    expect(currentButton).toBeDisabled()
  })

  it('renders single ancestor chain correctly', () => {
    const ancestors: AncestorSession[] = [
      { id: 'root', title: 'Root Session', isRoot: true },
      { id: 'current', title: 'Forked', isCurrent: true },
    ]
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current"
        forkDepth={1}
      />
    )
    expect(screen.getByText('Fork Ancestry (Level 1)')).toBeInTheDocument()
    expect(screen.getByText('Root Session')).toBeInTheDocument()
    expect(screen.getByText('Forked')).toBeInTheDocument()
  })

  it('handles untitled sessions', () => {
    const ancestors: AncestorSession[] = [
      { id: 'root', title: '', isRoot: true },
    ]
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="root"
        forkDepth={0}
      />
    )
    expect(screen.getByText('Untitled Session')).toBeInTheDocument()
  })

  it('navigates to root session when clicked', () => {
    const mockNavigate = vi.fn()
    const ancestors = createMockAncestors()
    render(
      <ForkAncestryTree
        ancestors={ancestors}
        currentSessionId="current-session"
        forkDepth={2}
        onNavigateToSession={mockNavigate}
      />
    )

    // Click on root session
    fireEvent.click(screen.getByText('Original Session').closest('button')!)
    expect(mockNavigate).toHaveBeenCalledWith('root-session')
  })
})
