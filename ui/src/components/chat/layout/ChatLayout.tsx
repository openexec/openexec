/**
 * ChatLayout Component
 *
 * Main container that manages the overall layout grid for the chat UI.
 * Provides responsive layout with collapsible sidebars.
 *
 * @module components/chat/layout/ChatLayout
 */

import React, { useState, useCallback } from 'react'

export interface ChatLayoutProps {
  /** Left sidebar content (session list) */
  sidebar?: React.ReactNode
  /** Main chat content area */
  children: React.ReactNode
  /** Right panel content (events, cost) */
  rightPanel?: React.ReactNode
  /** Whether left sidebar is visible by default */
  defaultSidebarOpen?: boolean
  /** Whether right panel is visible by default */
  defaultRightPanelOpen?: boolean
  /** Callback when sidebar visibility changes */
  onSidebarToggle?: (isOpen: boolean) => void
  /** Callback when right panel visibility changes */
  onRightPanelToggle?: (isOpen: boolean) => void
}

/**
 * Main chat layout container
 *
 * Layout structure:
 * - Left: Session sidebar (collapsible)
 * - Center: Chat main content
 * - Right: Event/Cost panels (collapsible)
 */
const ChatLayout: React.FC<ChatLayoutProps> = ({
  sidebar,
  children,
  rightPanel,
  defaultSidebarOpen = true,
  defaultRightPanelOpen = false,
  onSidebarToggle,
  onRightPanelToggle,
}) => {
  const [sidebarOpen, setSidebarOpen] = useState(defaultSidebarOpen)
  const [rightPanelOpen, setRightPanelOpen] = useState(defaultRightPanelOpen)

  const toggleSidebar = useCallback(() => {
    setSidebarOpen((prev) => {
      const next = !prev
      onSidebarToggle?.(next)
      return next
    })
  }, [onSidebarToggle])

  const toggleRightPanel = useCallback(() => {
    setRightPanelOpen((prev) => {
      const next = !prev
      onRightPanelToggle?.(next)
      return next
    })
  }, [onRightPanelToggle])

  return (
    <div className="chat-layout" style={layoutStyles.container}>
      {/* Sidebar toggle button (visible when collapsed) */}
      {!sidebarOpen && (
        <button
          className="chat-layout__sidebar-toggle"
          style={layoutStyles.sidebarToggle}
          onClick={toggleSidebar}
          aria-label="Open sidebar"
          title="Open sidebar"
        >
          <ChevronRightIcon />
        </button>
      )}

      {/* Left sidebar */}
      {sidebar && sidebarOpen && (
        <aside className="chat-layout__sidebar" style={layoutStyles.sidebar}>
          <div className="chat-layout__sidebar-header" style={layoutStyles.sidebarHeader}>
            <button
              className="chat-layout__collapse-btn"
              style={layoutStyles.collapseBtn}
              onClick={toggleSidebar}
              aria-label="Collapse sidebar"
              title="Collapse sidebar"
            >
              <ChevronLeftIcon />
            </button>
          </div>
          <div className="chat-layout__sidebar-content" style={layoutStyles.sidebarContent}>
            {sidebar}
          </div>
        </aside>
      )}

      {/* Main content area */}
      <main className="chat-layout__main" style={layoutStyles.main}>
        {children}

        {/* Right panel toggle */}
        {rightPanel && (
          <button
            className="chat-layout__right-toggle"
            style={layoutStyles.rightToggle}
            onClick={toggleRightPanel}
            aria-label={rightPanelOpen ? 'Close panel' : 'Open panel'}
            title={rightPanelOpen ? 'Close panel' : 'Open panel'}
          >
            {rightPanelOpen ? <ChevronRightIcon /> : <ChevronLeftIcon />}
          </button>
        )}
      </main>

      {/* Right panel */}
      {rightPanel && rightPanelOpen && (
        <aside className="chat-layout__right-panel" style={layoutStyles.rightPanel}>
          <div className="chat-layout__right-header" style={layoutStyles.rightHeader}>
            <button
              className="chat-layout__collapse-btn"
              style={layoutStyles.collapseBtn}
              onClick={toggleRightPanel}
              aria-label="Collapse panel"
              title="Collapse panel"
            >
              <ChevronRightIcon />
            </button>
          </div>
          <div className="chat-layout__right-content" style={layoutStyles.rightContent}>
            {rightPanel}
          </div>
        </aside>
      )}
    </div>
  )
}

// Icon components
const ChevronLeftIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="15 18 9 12 15 6" />
  </svg>
)

const ChevronRightIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="9 18 15 12 9 6" />
  </svg>
)

// Inline styles (can be replaced with CSS modules or styled-components)
const layoutStyles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    height: '100vh',
    width: '100%',
    backgroundColor: '#0d1117',
    color: '#c9d1d9',
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif',
    position: 'relative',
  },
  sidebar: {
    width: '280px',
    minWidth: '280px',
    borderRight: '1px solid #30363d',
    backgroundColor: '#161b22',
    display: 'flex',
    flexDirection: 'column',
  },
  sidebarHeader: {
    display: 'flex',
    justifyContent: 'flex-end',
    padding: '8px',
    borderBottom: '1px solid #30363d',
  },
  sidebarContent: {
    flex: 1,
    overflow: 'auto',
  },
  sidebarToggle: {
    position: 'absolute',
    left: '8px',
    top: '50%',
    transform: 'translateY(-50%)',
    width: '28px',
    height: '48px',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '0 6px 6px 0',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#c9d1d9',
    zIndex: 10,
  },
  collapseBtn: {
    backgroundColor: 'transparent',
    border: 'none',
    cursor: 'pointer',
    padding: '4px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#8b949e',
    borderRadius: '4px',
  },
  main: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    position: 'relative',
  },
  rightToggle: {
    position: 'absolute',
    right: '8px',
    top: '50%',
    transform: 'translateY(-50%)',
    width: '28px',
    height: '48px',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '6px 0 0 6px',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#c9d1d9',
    zIndex: 10,
  },
  rightPanel: {
    width: '320px',
    minWidth: '320px',
    borderLeft: '1px solid #30363d',
    backgroundColor: '#161b22',
    display: 'flex',
    flexDirection: 'column',
  },
  rightHeader: {
    display: 'flex',
    justifyContent: 'flex-start',
    padding: '8px',
    borderBottom: '1px solid #30363d',
  },
  rightContent: {
    flex: 1,
    overflow: 'auto',
  },
}

export default ChatLayout
