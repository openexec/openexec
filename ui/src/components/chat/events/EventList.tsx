/**
 * EventList Component
 *
 * Virtualized list of events with auto-scroll and color coding.
 *
 * @module components/chat/events/EventList
 */

import React, { useEffect, useRef, useCallback } from 'react'
import type { LoopEvent } from '../../../types/chat'
import EventItem from './EventItem'

export interface EventListProps {
  /** List of events to display */
  events: LoopEvent[]
  /** Whether to auto-scroll to newest events */
  autoScroll?: boolean
  /** Whether new events are being received (for scroll behavior) */
  isLive?: boolean
}

const EventList: React.FC<EventListProps> = ({
  events,
  autoScroll = true,
  isLive = false,
}) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const isUserScrolling = useRef(false)
  const lastEventCount = useRef(events.length)

  // Handle scroll - detect if user is scrolling away from bottom
  const handleScroll = useCallback(() => {
    if (!containerRef.current) return

    const { scrollTop, scrollHeight, clientHeight } = containerRef.current
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50

    // If user scrolled away from bottom, disable auto-scroll
    isUserScrolling.current = !isAtBottom
  }, [])

  // Auto-scroll when new events arrive
  useEffect(() => {
    if (!autoScroll || isUserScrolling.current) return

    // Only scroll if we have new events
    if (events.length > lastEventCount.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
    lastEventCount.current = events.length
  }, [events.length, autoScroll])

  // Scroll to bottom when switching to live mode
  useEffect(() => {
    if (isLive && autoScroll) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
      isUserScrolling.current = false
    }
  }, [isLive, autoScroll])

  // Scroll to bottom button handler
  const scrollToBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    isUserScrolling.current = false
  }, [])

  // Show scroll-to-bottom button when user has scrolled up
  const showScrollButton = isUserScrolling.current && events.length > 0

  return (
    <div className="event-list" style={styles.wrapper}>
      <div
        ref={containerRef}
        className="event-list__container"
        style={styles.container}
        onScroll={handleScroll}
      >
        {/* Empty state */}
        {events.length === 0 && (
          <div className="event-list__empty" style={styles.empty}>
            <span style={styles.emptyIcon}>●</span>
            <p style={styles.emptyText}>No events yet</p>
            <p style={styles.emptyHint}>Events will appear here as the agent runs</p>
          </div>
        )}

        {/* Event items */}
        {events.map((event) => (
          <EventItem key={event.id} event={event} />
        ))}

        {/* Bottom anchor */}
        <div ref={bottomRef} className="event-list__bottom" />
      </div>

      {/* Scroll to bottom button */}
      {showScrollButton && (
        <button
          className="event-list__scroll-btn"
          style={styles.scrollButton}
          onClick={scrollToBottom}
          aria-label="Scroll to latest events"
          title="Scroll to latest events"
        >
          ↓ New events
        </button>
      )}

      {/* Live indicator */}
      {isLive && (
        <div className="event-list__live" style={styles.liveIndicator}>
          <span style={styles.liveDot} />
          Live
        </div>
      )}
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  wrapper: {
    position: 'relative',
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
  },
  container: {
    flex: 1,
    overflow: 'auto',
    scrollBehavior: 'smooth',
  },
  empty: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: '200px',
    padding: '24px',
    textAlign: 'center',
  },
  emptyIcon: {
    fontSize: '24px',
    color: '#30363d',
    marginBottom: '12px',
  },
  emptyText: {
    color: '#8b949e',
    fontSize: '14px',
    margin: 0,
  },
  emptyHint: {
    color: '#6e7681',
    fontSize: '12px',
    margin: '4px 0 0 0',
  },
  scrollButton: {
    position: 'absolute',
    bottom: '16px',
    left: '50%',
    transform: 'translateX(-50%)',
    backgroundColor: '#238636',
    color: '#ffffff',
    border: 'none',
    borderRadius: '16px',
    padding: '6px 16px',
    fontSize: '12px',
    fontWeight: 500,
    cursor: 'pointer',
    boxShadow: '0 2px 8px rgba(0, 0, 0, 0.3)',
    zIndex: 1,
  },
  liveIndicator: {
    position: 'absolute',
    top: '8px',
    right: '8px',
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    backgroundColor: 'rgba(35, 134, 54, 0.2)',
    color: '#3fb950',
    fontSize: '10px',
    fontWeight: 500,
    padding: '2px 8px',
    borderRadius: '10px',
  },
  liveDot: {
    width: '6px',
    height: '6px',
    backgroundColor: '#3fb950',
    borderRadius: '50%',
    animation: 'pulse 2s infinite',
  },
}

export default EventList
