/**
 * MessageContent Component
 *
 * Renders message content with basic markdown support.
 * Handles code blocks, inline code, and links.
 *
 * @module components/chat/messages/MessageContent
 */

import React from 'react'

export interface MessageContentProps {
  /** The raw content string */
  content: string
}

/**
 * Simple markdown-like rendering for chat messages.
 * Supports: code blocks, inline code, links, bold, italic
 */
const MessageContent: React.FC<MessageContentProps> = ({ content }) => {
  const rendered = React.useMemo(() => {
    return renderContent(content)
  }, [content])

  return (
    <div className="message-content" style={styles.container}>
      {rendered}
    </div>
  )
}

/**
 * Parse and render content with markdown support
 */
function renderContent(content: string): React.ReactNode[] {
  const result: React.ReactNode[] = []
  const lines = content.split('\n')
  let inCodeBlock = false
  let codeBlockContent: string[] = []
  let codeBlockLang = ''
  let keyIndex = 0

  for (const line of lines) {
    // Code block start/end
    if (line.startsWith('```')) {
      if (inCodeBlock) {
        // End code block
        result.push(
          <CodeBlock
            key={`code-${keyIndex++}`}
            code={codeBlockContent.join('\n')}
            language={codeBlockLang}
          />
        )
        codeBlockContent = []
        codeBlockLang = ''
        inCodeBlock = false
      } else {
        // Start code block
        codeBlockLang = line.slice(3).trim()
        inCodeBlock = true
      }
      continue
    }

    if (inCodeBlock) {
      codeBlockContent.push(line)
      continue
    }

    // Regular line - parse inline elements
    result.push(
      <p key={`p-${keyIndex++}`} style={styles.paragraph}>
        {renderInline(line)}
      </p>
    )
  }

  // Handle unclosed code block
  if (inCodeBlock && codeBlockContent.length > 0) {
    result.push(
      <CodeBlock
        key={`code-${keyIndex++}`}
        code={codeBlockContent.join('\n')}
        language={codeBlockLang}
      />
    )
  }

  return result
}

/**
 * Render inline elements (code, links, bold, italic)
 */
function renderInline(text: string): React.ReactNode[] {
  const result: React.ReactNode[] = []
  let remaining = text
  let keyIndex = 0

  while (remaining.length > 0) {
    // Inline code
    const codeMatch = remaining.match(/^`([^`]+)`/)
    if (codeMatch) {
      result.push(
        <code key={`ic-${keyIndex++}`} style={styles.inlineCode}>
          {codeMatch[1]}
        </code>
      )
      remaining = remaining.slice(codeMatch[0].length)
      continue
    }

    // Link
    const linkMatch = remaining.match(/^\[([^\]]+)\]\(([^)]+)\)/)
    if (linkMatch) {
      result.push(
        <a
          key={`link-${keyIndex++}`}
          href={linkMatch[2]}
          target="_blank"
          rel="noopener noreferrer"
          style={styles.link}
        >
          {linkMatch[1]}
        </a>
      )
      remaining = remaining.slice(linkMatch[0].length)
      continue
    }

    // Bold
    const boldMatch = remaining.match(/^\*\*([^*]+)\*\*/)
    if (boldMatch) {
      result.push(
        <strong key={`bold-${keyIndex++}`} style={styles.bold}>
          {boldMatch[1]}
        </strong>
      )
      remaining = remaining.slice(boldMatch[0].length)
      continue
    }

    // Italic
    const italicMatch = remaining.match(/^\*([^*]+)\*/)
    if (italicMatch) {
      result.push(
        <em key={`em-${keyIndex++}`} style={styles.italic}>
          {italicMatch[1]}
        </em>
      )
      remaining = remaining.slice(italicMatch[0].length)
      continue
    }

    // Regular text until next special character
    const nextSpecial = remaining.search(/[`\[\*]/)
    if (nextSpecial === -1) {
      result.push(remaining)
      break
    } else if (nextSpecial === 0) {
      // Special char but didn't match - treat as regular text
      result.push(remaining[0])
      remaining = remaining.slice(1)
    } else {
      result.push(remaining.slice(0, nextSpecial))
      remaining = remaining.slice(nextSpecial)
    }
  }

  return result
}

/**
 * Code block component with optional language
 */
const CodeBlock: React.FC<{ code: string; language?: string }> = ({ code, language }) => (
  <div className="code-block" style={styles.codeBlock}>
    {language && (
      <div className="code-block__header" style={styles.codeHeader}>
        {language}
      </div>
    )}
    <pre style={styles.pre}>
      <code style={styles.code}>{code}</code>
    </pre>
  </div>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    fontSize: '14px',
    lineHeight: 1.6,
    color: '#c9d1d9',
    wordBreak: 'break-word',
  },
  paragraph: {
    margin: '0 0 8px 0',
  },
  inlineCode: {
    backgroundColor: '#30363d',
    padding: '2px 6px',
    borderRadius: '4px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: '13px',
    color: '#f0883e',
  },
  link: {
    color: '#58a6ff',
    textDecoration: 'none',
  },
  bold: {
    fontWeight: 600,
    color: '#e6edf3',
  },
  italic: {
    fontStyle: 'italic',
  },
  codeBlock: {
    backgroundColor: '#161b22',
    borderRadius: '6px',
    overflow: 'hidden',
    margin: '8px 0',
    border: '1px solid #30363d',
  },
  codeHeader: {
    backgroundColor: '#21262d',
    padding: '6px 12px',
    fontSize: '12px',
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  pre: {
    margin: 0,
    padding: '12px',
    overflow: 'auto',
  },
  code: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: '13px',
    color: '#c9d1d9',
    whiteSpace: 'pre',
  },
}

export default MessageContent
