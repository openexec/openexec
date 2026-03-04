/**
 * App Component
 *
 * Root application component that sets up the chat UI with session management.
 * Provides the main layout structure and handles configuration.
 *
 * @module App
 */

import React from 'react'
import ChatPage from './pages/ChatPage'

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
const defaultConfig: AppConfig = {
  wsUrl: import.meta.env.VITE_WS_URL ?? 'ws://localhost:8765/ws',
  apiUrl: '/api',
  authToken: import.meta.env.VITE_AUTH_TOKEN,
  debug: import.meta.env.DEV,
}

/**
 * App Component Props
 */
export interface AppProps {
  /** Optional config override */
  config?: Partial<AppConfig>
}

/**
 * Root Application Component
 */
const App: React.FC<AppProps> = ({ config }) => {
  const mergedConfig = { ...defaultConfig, ...config }

  return <ChatPage config={mergedConfig} />
}

export default App
