/**
 * App Component
 *
 * Root application component that sets up the chat UI with session management.
 * Provides the main layout structure and handles configuration.
 *
 * @module App
 */

import React, { useState } from 'react'
import ChatPage from './pages/ChatPage'
import KnowledgeHub from './pages/KnowledgeHub'
import UsageDashboard from './pages/UsageDashboard'

// Placeholder for ReplayPage until created in a separate file
const ReplayPage = () => <div style={{ padding: '2rem', color: '#c9d1d9' }}>Replay Tooling (v2) - Visualizing audit run steps...</div>

/**
 * Application configuration
 */
export interface AppConfig {
  /** WebSocket server URL */
  wsUrl: string
  /** REST API base URL */
  apiUrl: string
  /** Optional auth token */
  authToken?: string
  /** Enable debug mode */
  debug?: boolean
}

/**
 * Default configuration from environment or fallback values
 */
const inferWsUrl = (): string => {
  const envWs = import.meta.env.VITE_WS_URL as string | undefined
  if (envWs) return envWs
  if (typeof window !== 'undefined') {
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${window.location.host}/ws`
  }
  return 'ws://127.0.0.1:8080/ws'
}

const defaultConfig: AppConfig = {
  wsUrl: inferWsUrl(),
  apiUrl: (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api',
  authToken: import.meta.env.VITE_AUTH_TOKEN as string | undefined,
  debug: import.meta.env.DEV,
}

/**
 * App Component Props
 */
export interface AppProps {
  /** Optional config override */
  config?: Partial<AppConfig>
}

const navStyles = {
  nav: {
    backgroundColor: '#161b22',
    padding: '0.5rem 1rem',
    borderBottom: '1px solid #30363d',
    display: 'flex',
    gap: '1rem',
  },
  btn: {
    background: 'none',
    border: 'none',
    color: '#c9d1d9',
    cursor: 'pointer',
    padding: '0.5rem',
    fontSize: '14px',
    fontWeight: 500,
  },
  activeBtn: {
    color: '#58a6ff',
    borderBottom: '2px solid #58a6ff',
  }
}

/**
 * Root Application Component
 */
const App: React.FC<AppProps> = ({ config }) => {
  const mergedConfig = { ...defaultConfig, ...config }
  const [view, setView] = useState<'chat' | 'knowledge' | 'usage' | 'replay'>('chat')

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '100vw' }}>
      <nav style={navStyles.nav}>
        <button 
          style={{ ...navStyles.btn, ...(view === 'chat' ? navStyles.activeBtn : {}) }}
          onClick={() => setView('chat')}
        >
          Chat
        </button>
        <button 
          style={{ ...navStyles.btn, ...(view === 'knowledge' ? navStyles.activeBtn : {}) }}
          onClick={() => setView('knowledge')}
        >
          Knowledge Hub
        </button>
        <button 
          style={{ ...navStyles.btn, ...(view === 'usage' ? navStyles.activeBtn : {}) }}
          onClick={() => setView('usage')}
        >
          Usage
        </button>
        <button 
          style={{ ...navStyles.btn, ...(view === 'replay' ? navStyles.activeBtn : {}) }}
          onClick={() => setView('replay')}
        >
          Replay
        </button>
      </nav>
      <div style={{ flex: 1, overflow: 'hidden' }}>
        {view === 'chat' ? (
          <ChatPage config={mergedConfig} />
        ) : view === 'knowledge' ? (
          <KnowledgeHub />
        ) : view === 'usage' ? (
          <UsageDashboard config={{ apiBaseUrl: mergedConfig.apiUrl }} />
        ) : (
          <ReplayPage />
        )}
      </div>
    </div>
  )
}

export default App
