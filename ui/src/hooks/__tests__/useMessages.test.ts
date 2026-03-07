/**
 * Tests for useMessages hook
 *
 * @module hooks/__tests__/useMessages.test
 */

import { renderHook, act, waitFor } from '@testing-library/react'
import { useMessages, MessagesApiConfig } from '../useMessages'

// Mock fetch
const mockFetch = jest.fn()
global.fetch = mockFetch

describe('useMessages', () => {
  const defaultConfig: MessagesApiConfig = {
    baseUrl: 'http://localhost:8080/api',
    authToken: 'test-token',
  }

  beforeEach(() => {
    mockFetch.mockClear()
  })

  describe('fetchMessages', () => {
    it('should fetch messages successfully', async () => {
      const messagesResponse = {
        messages: [
          {
            id: 'msg-1',
            sessionId: 'session-1',
            role: 'user',
            content: 'Hello',
            tokensInput: 5,
            tokensOutput: 0,
            costUsd: 0,
            createdAt: '2024-01-01T00:00:00Z',
          },
          {
            id: 'msg-2',
            sessionId: 'session-1',
            role: 'assistant',
            content: 'Hi there!',
            tokensInput: 0,
            tokensOutput: 10,
            costUsd: 0.001,
            createdAt: '2024-01-01T00:00:01Z',
          },
        ],
        pagination: {
          offset: 0,
          limit: 50,
          hasMore: false,
          totalCount: 2,
        },
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(messagesResponse),
      })

      const { result } = renderHook(() => useMessages(defaultConfig))

      await act(async () => {
        await result.current.fetchMessages('session-1')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/sessions/session-1/messages?limit=50&offset=0',
        expect.objectContaining({
          method: 'GET',
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        })
      )

      expect(result.current.messages).toEqual(messagesResponse.messages)
      expect(result.current.pagination).toEqual(messagesResponse.pagination)
      expect(result.current.messagesLoading).toBe(false)
    })

    it('should handle fetch error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: async () => 'Session not found',
      })

      const { result } = renderHook(() => useMessages(defaultConfig))

      await act(async () => {
        await result.current.fetchMessages('session-missing')
      })

      expect(result.current.messagesError).toBe('API error 404: Session not found')
    })

    it('should append messages when append option is true', async () => {
      const firstResponse = {
        messages: [
          {
            id: 'msg-1',
            sessionId: 'session-1',
            role: 'user',
            content: 'First',
            tokensInput: 5,
            tokensOutput: 0,
            costUsd: 0,
            createdAt: '2024-01-01T00:00:00Z',
          },
        ],
        pagination: {
          offset: 0,
          limit: 1,
          hasMore: true,
          totalCount: 2,
        },
      }

      const secondResponse = {
        messages: [
          {
            id: 'msg-2',
            sessionId: 'session-1',
            role: 'assistant',
            content: 'Second',
            tokensInput: 0,
            tokensOutput: 5,
            costUsd: 0.001,
            createdAt: '2024-01-01T00:00:01Z',
          },
        ],
        pagination: {
          offset: 1,
          limit: 1,
          hasMore: false,
          totalCount: 2,
        },
      }

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(firstResponse),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(secondResponse),
        })

      const { result } = renderHook(() => useMessages(defaultConfig))

      await act(async () => {
        await result.current.fetchMessages('session-1', { limit: 1 })
      })

      expect(result.current.messages).toHaveLength(1)

      await act(async () => {
        await result.current.fetchMessages('session-1', {
          limit: 1,
          offset: 1,
          append: true,
        })
      })

      expect(result.current.messages).toHaveLength(2)
      expect(result.current.messages[0].id).toBe('msg-1')
      expect(result.current.messages[1].id).toBe('msg-2')
    })
  })

  describe('loadMoreMessages', () => {
    it('should load more messages when hasMore is true', async () => {
      const firstResponse = {
        messages: [
          {
            id: 'msg-1',
            sessionId: 'session-1',
            role: 'user',
            content: 'First',
            tokensInput: 5,
            tokensOutput: 0,
            costUsd: 0,
            createdAt: '2024-01-01T00:00:00Z',
          },
        ],
        pagination: {
          offset: 0,
          limit: 50,
          hasMore: true,
          totalCount: 100,
        },
      }

      const secondResponse = {
        messages: [
          {
            id: 'msg-2',
            sessionId: 'session-1',
            role: 'assistant',
            content: 'Second',
            tokensInput: 0,
            tokensOutput: 5,
            costUsd: 0.001,
            createdAt: '2024-01-01T00:00:01Z',
          },
        ],
        pagination: {
          offset: 50,
          limit: 50,
          hasMore: false,
          totalCount: 100,
        },
      }

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(firstResponse),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(secondResponse),
        })

      const { result } = renderHook(() => useMessages(defaultConfig))

      await act(async () => {
        await result.current.fetchMessages('session-1')
      })

      await act(async () => {
        await result.current.loadMoreMessages('session-1')
      })

      expect(result.current.messages).toHaveLength(2)
    })

    it('should not load more when hasMore is false', async () => {
      const response = {
        messages: [],
        pagination: {
          offset: 0,
          limit: 50,
          hasMore: false,
          totalCount: 0,
        },
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(response),
      })

      const { result } = renderHook(() => useMessages(defaultConfig))

      await act(async () => {
        await result.current.fetchMessages('session-1')
      })

      await act(async () => {
        await result.current.loadMoreMessages('session-1')
      })

      // Should not make another fetch call
      expect(mockFetch).toHaveBeenCalledTimes(1)
    })
  })

  describe('addMessage', () => {
    it('should add a new message', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      const newMessage = {
        id: 'msg-new',
        sessionId: 'session-1',
        role: 'user' as const,
        content: 'New message',
        tokensInput: 5,
        tokensOutput: 0,
        costUsd: 0,
        createdAt: '2024-01-01T00:00:00Z',
      }

      act(() => {
        result.current.addMessage(newMessage)
      })

      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0]).toEqual(newMessage)
    })

    it('should update existing message with same ID', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      const originalMessage = {
        id: 'msg-1',
        sessionId: 'session-1',
        role: 'assistant' as const,
        content: 'Original',
        tokensInput: 0,
        tokensOutput: 5,
        costUsd: 0.001,
        createdAt: '2024-01-01T00:00:00Z',
      }

      const updatedMessage = {
        ...originalMessage,
        content: 'Updated',
        tokensOutput: 10,
        costUsd: 0.002,
      }

      act(() => {
        result.current.addMessage(originalMessage)
      })

      act(() => {
        result.current.addMessage(updatedMessage)
      })

      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0].content).toBe('Updated')
      expect(result.current.messages[0].tokensOutput).toBe(10)
    })
  })

  describe('handleStreamingChunk', () => {
    it('should start a new streaming message', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      act(() => {
        result.current.handleStreamingChunk({
          messageId: 'msg-streaming',
          content: 'Hello',
          isComplete: false,
        })
      })

      expect(result.current.streamingMessage).toBeDefined()
      expect(result.current.streamingMessage?.id).toBe('msg-streaming')
      expect(result.current.streamingMessage?.content).toBe('Hello')
    })

    it('should append to existing streaming message', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      act(() => {
        result.current.handleStreamingChunk({
          messageId: 'msg-streaming',
          content: 'Hello',
          isComplete: false,
        })
      })

      act(() => {
        result.current.handleStreamingChunk({
          messageId: 'msg-streaming',
          content: ' World',
          isComplete: false,
        })
      })

      expect(result.current.streamingMessage?.content).toBe('Hello World')
    })

    it('should handle tool calls in streaming chunk', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      const toolCall = {
        id: 'tc-1',
        messageId: 'msg-streaming',
        sessionId: 'session-1',
        toolName: 'read_file',
        toolInput: '{"path": "/test.txt"}',
        status: 'pending' as const,
        createdAt: '2024-01-01T00:00:00Z',
      }

      act(() => {
        result.current.handleStreamingChunk({
          messageId: 'msg-streaming',
          content: 'Let me read that file',
          isComplete: false,
          toolCall,
        })
      })

      expect(result.current.streamingMessage?.toolCalls).toBeDefined()
      expect(result.current.streamingMessage?.toolCalls).toHaveLength(1)
      expect(result.current.streamingMessage?.toolCalls?.[0].toolName).toBe('read_file')
    })
  })

  describe('completeStreamingMessage', () => {
    it('should complete streaming message and add to messages', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      act(() => {
        result.current.handleStreamingChunk({
          messageId: 'msg-streaming',
          content: 'Hello',
          isComplete: false,
        })
      })

      const completedMessage = {
        id: 'msg-streaming',
        sessionId: 'session-1',
        role: 'assistant' as const,
        content: 'Hello World',
        tokensInput: 0,
        tokensOutput: 10,
        costUsd: 0.001,
        createdAt: '2024-01-01T00:00:00Z',
      }

      act(() => {
        result.current.completeStreamingMessage(completedMessage)
      })

      expect(result.current.streamingMessage).toBeUndefined()
      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0]).toEqual(completedMessage)
    })
  })

  describe('clearMessages', () => {
    it('should clear all messages and streaming message', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      act(() => {
        result.current.addMessage({
          id: 'msg-1',
          sessionId: 'session-1',
          role: 'user',
          content: 'Hello',
          tokensInput: 5,
          tokensOutput: 0,
          costUsd: 0,
          createdAt: '2024-01-01T00:00:00Z',
        })
      })

      act(() => {
        result.current.handleStreamingChunk({
          messageId: 'msg-streaming',
          content: 'Response',
          isComplete: false,
        })
      })

      act(() => {
        result.current.clearMessages()
      })

      expect(result.current.messages).toHaveLength(0)
      expect(result.current.streamingMessage).toBeUndefined()
    })
  })

  describe('getSortedMessages', () => {
    it('should return messages sorted by createdAt', () => {
      const { result } = renderHook(() => useMessages(defaultConfig))

      // Add messages out of order
      act(() => {
        result.current.addMessage({
          id: 'msg-2',
          sessionId: 'session-1',
          role: 'assistant',
          content: 'Second',
          tokensInput: 0,
          tokensOutput: 5,
          costUsd: 0.001,
          createdAt: '2024-01-01T00:00:02Z',
        })
      })

      act(() => {
        result.current.addMessage({
          id: 'msg-1',
          sessionId: 'session-1',
          role: 'user',
          content: 'First',
          tokensInput: 5,
          tokensOutput: 0,
          costUsd: 0,
          createdAt: '2024-01-01T00:00:01Z',
        })
      })

      act(() => {
        result.current.addMessage({
          id: 'msg-3',
          sessionId: 'session-1',
          role: 'user',
          content: 'Third',
          tokensInput: 3,
          tokensOutput: 0,
          costUsd: 0,
          createdAt: '2024-01-01T00:00:03Z',
        })
      })

      const sorted = result.current.getSortedMessages()

      expect(sorted[0].id).toBe('msg-1')
      expect(sorted[1].id).toBe('msg-2')
      expect(sorted[2].id).toBe('msg-3')
    })
  })
})
