/**
 * ProjectWizard Component
 *
 * Guided intent interviewer for bootstrapping new projects.
 *
 * @module components/chat/session/ProjectWizard
 */

import React, { useState, useCallback, useEffect, useRef } from 'react'

export interface ProjectWizardProps {
  /** Project path to run wizard in */
  projectPath: string
  /** Callback to close wizard */
  onClose: () => void
  /** Base API URL */
  apiUrl: string
}

interface WizardResponse {
  updated_state: any
  next_question: string
  acknowledgement: string
  is_complete: boolean
  new_facts: string[]
  new_assumptions: string[]
}

const ProjectWizard: React.FC<ProjectWizardProps> = ({ projectPath, onClose, apiUrl }) => {
  const [messages, setMessages] = useState<{role: 'assistant' | 'user', content: string}[]>([])
  const [inputValue, setInputValue] = useState('')
  const [state, setState] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [isComplete, setIsComplete] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  // Auto-scroll
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  // Start wizard
  useEffect(() => {
    handleSendMessage('start')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleSendMessage = async (msg: string) => {
    if (loading) return
    
    if (msg !== 'start') {
      setMessages(prev => [...prev, { role: 'user', content: msg }])
    }
    
    setLoading(true)
    setInputValue('')

    try {
      const response = await fetch(`${apiUrl}/projects/wizard`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          projectPath,
          message: msg,
          state,
          model: 'sonnet', // Default
        })
      })

      if (!response.ok) throw new Error('Wizard failed')
      
      const data: WizardResponse = await response.json()
      
      // Update state
      setState(JSON.stringify(data.updated_state))
      
      // Build response content
      let content = data.acknowledgement ? `${data.acknowledgement}

` : ''
      if (data.next_question) content += data.next_question
      
      setMessages(prev => [...prev, { role: 'assistant', content }])
      
      if (data.is_complete) {
        setIsComplete(true)
      }
    } catch (err) {
      setMessages(prev => [...prev, { role: 'assistant', content: 'Error: Failed to connect to wizard.' }])
    } finally {
      setLoading(false)
    }
  }

  const handleRenderIntent = async () => {
    setLoading(true)
    try {
      const response = await fetch(`${apiUrl}/projects/wizard`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          projectPath,
          state,
          render: true,
        })
      })
      
      if (response.ok) {
        onClose()
      }
    } catch (err) {
      console.error('Render failed', err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <div style={styles.header}>
          <h3 style={styles.title}>Project Setup Wizard</h3>
          <button onClick={onClose} style={styles.closeButton}>×</button>
        </div>

        <div style={styles.chatContainer} ref={scrollRef}>
          {messages.map((m, i) => (
            <div key={i} style={m.role === 'assistant' ? styles.assistantMsg : styles.userMsg}>
              <div style={styles.msgBubble}>
                {m.content.split('\n').map((line, j) => (
                  <div key={j}>{line || <br />}</div>
                ))}
              </div>
            </div>
          ))}
          {loading && <div style={styles.loading}>Thinking...</div>}
        </div>

        <div style={styles.footer}>
          {isComplete ? (
            <div style={styles.completeActions}>
              <p style={styles.completeText}>Intent is complete! Ready to generate INTENT.md?</p>
              <button onClick={handleRenderIntent} style={styles.renderButton}>Generate INTENT.md</button>
            </div>
          ) : (
            <form 
              onSubmit={e => { e.preventDefault(); if (inputValue.trim()) handleSendMessage(inputValue.trim()) }}
              style={styles.inputForm}
            >
              <input
                value={inputValue}
                onChange={e => setInputValue(e.target.value)}
                placeholder="Tell me about your project..."
                style={styles.input}
                disabled={loading}
              />
              <button type="submit" disabled={loading || !inputValue.trim()} style={styles.sendButton}>
                Send
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    top: 0, left: 0, right: 0, bottom: 0,
    backgroundColor: 'rgba(0,0,0,0.7)',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    zIndex: 2000,
  },
  modal: {
    backgroundColor: '#161b22',
    width: '600px', height: '80vh',
    borderRadius: '12px', border: '1px solid #30363d',
    display: 'flex', flexDirection: 'column',
    overflow: 'hidden',
  },
  header: {
    padding: '16px', borderBottom: '1px solid #30363d',
    display: 'flex', justifyContent: 'space-between', alignItems: 'center',
  },
  title: { margin: 0, color: '#c9d1d9', fontSize: '16px' },
  closeButton: { background: 'none', border: 'none', color: '#8b949e', fontSize: '24px', cursor: 'pointer' },
  chatContainer: {
    flex: 1, padding: '20px', overflowY: 'auto',
    display: 'flex', flexDirection: 'column', gap: '16px',
  },
  assistantMsg: { alignSelf: 'flex-start', maxWidth: '85%' },
  userMsg: { alignSelf: 'flex-end', maxWidth: '85%' },
  msgBubble: {
    padding: '12px 16px', borderRadius: '12px',
    backgroundColor: '#21262d', color: '#c9d1d9',
    fontSize: '14px', lineHeight: '1.5',
  },
  footer: { padding: '16px', borderTop: '1px solid #30363d' },
  inputForm: { display: 'flex', gap: '8px' },
  input: {
    flex: 1, backgroundColor: '#0d1117', border: '1px solid #30363d',
    borderRadius: '6px', padding: '8px 12px', color: '#c9d1d9', outline: 'none',
  },
  sendButton: {
    backgroundColor: '#238636', color: '#fff', border: 'none',
    borderRadius: '6px', padding: '8px 16px', cursor: 'pointer',
  },
  loading: { color: '#8b949e', fontSize: '12px', fontStyle: 'italic' },
  completeActions: { textAlign: 'center' },
  completeText: { color: '#c9d1d9', marginBottom: '12px' },
  renderButton: {
    backgroundColor: '#238636', color: '#fff', border: 'none',
    borderRadius: '6px', padding: '10px 20px', cursor: 'pointer', fontWeight: 'bold',
  },
}

export default ProjectWizard
