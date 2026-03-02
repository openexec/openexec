/**
 * ChatLayout Component Tests
 *
 * Tests for the main layout container with collapsible panels.
 *
 * @module components/chat/__tests__/ChatLayout.test
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ChatLayout from '../layout/ChatLayout'

describe('ChatLayout', () => {
  it('renders children in main content area', () => {
    render(
      <ChatLayout>
        <div data-testid="main-content">Main Content</div>
      </ChatLayout>
    )

    expect(screen.getByTestId('main-content')).toBeInTheDocument()
  })

  it('renders sidebar when provided and open by default', () => {
    render(
      <ChatLayout sidebar={<div data-testid="sidebar-content">Sidebar</div>}>
        <div>Main</div>
      </ChatLayout>
    )

    expect(screen.getByTestId('sidebar-content')).toBeInTheDocument()
  })

  it('hides sidebar when defaultSidebarOpen is false', () => {
    render(
      <ChatLayout
        sidebar={<div data-testid="sidebar-content">Sidebar</div>}
        defaultSidebarOpen={false}
      >
        <div>Main</div>
      </ChatLayout>
    )

    expect(screen.queryByTestId('sidebar-content')).not.toBeInTheDocument()
  })

  it('toggles sidebar visibility when collapse button is clicked', () => {
    render(
      <ChatLayout sidebar={<div data-testid="sidebar-content">Sidebar</div>}>
        <div>Main</div>
      </ChatLayout>
    )

    // Sidebar is visible initially
    expect(screen.getByTestId('sidebar-content')).toBeInTheDocument()

    // Click collapse button
    const collapseButton = screen.getByLabelText('Collapse sidebar')
    fireEvent.click(collapseButton)

    // Sidebar should be hidden
    expect(screen.queryByTestId('sidebar-content')).not.toBeInTheDocument()

    // Click open button
    const openButton = screen.getByLabelText('Open sidebar')
    fireEvent.click(openButton)

    // Sidebar should be visible again
    expect(screen.getByTestId('sidebar-content')).toBeInTheDocument()
  })

  it('calls onSidebarToggle when sidebar is toggled', () => {
    const onSidebarToggle = vi.fn()

    render(
      <ChatLayout
        sidebar={<div>Sidebar</div>}
        onSidebarToggle={onSidebarToggle}
      >
        <div>Main</div>
      </ChatLayout>
    )

    const collapseButton = screen.getByLabelText('Collapse sidebar')
    fireEvent.click(collapseButton)

    expect(onSidebarToggle).toHaveBeenCalledWith(false)
  })

  it('renders right panel when provided and toggled open', () => {
    render(
      <ChatLayout
        rightPanel={<div data-testid="right-panel">Right Panel</div>}
        defaultRightPanelOpen={true}
      >
        <div>Main</div>
      </ChatLayout>
    )

    expect(screen.getByTestId('right-panel')).toBeInTheDocument()
  })

  it('hides right panel by default', () => {
    render(
      <ChatLayout rightPanel={<div data-testid="right-panel">Right Panel</div>}>
        <div>Main</div>
      </ChatLayout>
    )

    expect(screen.queryByTestId('right-panel')).not.toBeInTheDocument()
  })

  it('toggles right panel when toggle button is clicked', () => {
    render(
      <ChatLayout rightPanel={<div data-testid="right-panel">Right Panel</div>}>
        <div>Main</div>
      </ChatLayout>
    )

    // Right panel is hidden initially
    expect(screen.queryByTestId('right-panel')).not.toBeInTheDocument()

    // Click open button
    const openButton = screen.getByLabelText('Open panel')
    fireEvent.click(openButton)

    // Right panel should be visible
    expect(screen.getByTestId('right-panel')).toBeInTheDocument()
  })

  it('calls onRightPanelToggle when right panel is toggled', () => {
    const onRightPanelToggle = vi.fn()

    render(
      <ChatLayout
        rightPanel={<div>Right Panel</div>}
        onRightPanelToggle={onRightPanelToggle}
      >
        <div>Main</div>
      </ChatLayout>
    )

    const openButton = screen.getByLabelText('Open panel')
    fireEvent.click(openButton)

    expect(onRightPanelToggle).toHaveBeenCalledWith(true)
  })
})
