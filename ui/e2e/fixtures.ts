import { test as base, expect, Page } from '@playwright/test'

/**
 * Custom test fixtures for OpenExec E2E tests
 */

/**
 * Extend the base test with custom fixtures
 */
export const test = base.extend<{
  /** Page with network requests mocked */
  mockedPage: Page
}>({
  mockedPage: async ({ page }, use) => {
    // Mock WebSocket connections to prevent real backend calls
    await page.route('**/ws', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'text/plain',
        body: '',
      })
    })

    // Mock API endpoints with empty responses
    await page.route('**/api/**', (route) => {
      const url = route.request().url()

      // Return appropriate mock data based on endpoint
      if (url.includes('/sessions')) {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([]),
        })
      } else {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({}),
        })
      }
    })

    await use(page)
  },
})

export { expect }

/**
 * Helper functions for E2E tests
 */
export const helpers = {
  /**
   * Wait for the app to be fully loaded
   */
  async waitForAppLoad(page: Page) {
    await page.waitForLoadState('domcontentloaded')
    await page.waitForSelector('#root:not(:empty)', { timeout: 10000 })
  },

  /**
   * Mock a session list response
   */
  mockSessionsResponse(sessions: Array<{ id: string; title: string; status?: string }>) {
    return sessions.map((s) => ({
      id: s.id,
      title: s.title,
      status: s.status ?? 'idle',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      messageCount: 0,
    }))
  },

  /**
   * Create a mock message object
   */
  mockMessage(role: 'user' | 'assistant', content: string, id?: string) {
    return {
      id: id ?? `msg-${Date.now()}`,
      sessionId: 'test-session',
      role,
      content,
      createdAt: new Date().toISOString(),
    }
  },

  /**
   * Create a mock session with full details
   */
  mockSession(params: {
    id: string
    title: string
    projectPath: string
    provider?: string
    model?: string
    status?: string
    parentSessionId?: string
    forkPointMessageId?: string
    messageCount?: number
    totalCostUsd?: number
  }) {
    return {
      id: params.id,
      title: params.title,
      projectPath: params.projectPath,
      provider: params.provider ?? 'anthropic',
      model: params.model ?? 'claude-3-5-sonnet-20241022',
      status: params.status ?? 'active',
      parentSessionId: params.parentSessionId,
      forkPointMessageId: params.forkPointMessageId,
      messageCount: params.messageCount ?? 0,
      totalCostUsd: params.totalCostUsd ?? 0,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }
  },

  /**
   * Create mock provider list
   */
  mockProviders() {
    return [
      {
        id: 'anthropic',
        name: 'Anthropic',
        models: [
          { id: 'claude-3-5-sonnet-20241022', name: 'Claude 3.5 Sonnet' },
          { id: 'claude-3-opus-20240229', name: 'Claude 3 Opus' },
        ],
        isAvailable: true,
      },
      {
        id: 'openai',
        name: 'OpenAI',
        models: [
          { id: 'gpt-4-turbo', name: 'GPT-4 Turbo' },
          { id: 'gpt-4o', name: 'GPT-4o' },
        ],
        isAvailable: true,
      },
      {
        id: 'gemini',
        name: 'Google Gemini',
        models: [{ id: 'gemini-1.5-pro', name: 'Gemini 1.5 Pro' }],
        isAvailable: true,
      },
    ]
  },

  /**
   * Setup comprehensive API mocking for session tests
   */
  async setupSessionMocks(
    page: Page,
    options: {
      sessions?: Array<ReturnType<typeof helpers.mockSession>>
      providers?: ReturnType<typeof helpers.mockProviders>
    } = {}
  ) {
    const sessions = options.sessions ?? []
    const providers = options.providers ?? helpers.mockProviders()

    // Mock WebSocket
    await page.route('**/ws', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'text/plain',
        body: '',
      })
    })

    // Mock providers endpoint
    await page.route('**/api/providers', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(providers),
      })
    })

    // Mock sessions list endpoint
    await page.route('**/api/sessions', (route) => {
      if (route.request().method() === 'GET') {
        const url = new URL(route.request().url())
        const projectPath = url.searchParams.get('project_path')

        const filteredSessions = projectPath
          ? sessions.filter((s) => s.projectPath === projectPath)
          : sessions

        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(filteredSessions),
        })
      } else if (route.request().method() === 'POST') {
        const postData = JSON.parse(route.request().postData() || '{}')
        const newSession = helpers.mockSession({
          id: `session-${Date.now()}`,
          title: postData.title || 'New Session',
          projectPath: postData.projectPath || '',
          provider: postData.provider,
          model: postData.model,
        })

        route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify(newSession),
        })
      }
    })

    // Mock individual session endpoints
    await page.route('**/api/sessions/*', (route) => {
      const url = route.request().url()
      const pathParts = url.split('/')
      const sessionIdOrAction = pathParts[pathParts.length - 1].split('?')[0]

      if (route.request().method() === 'GET') {
        const session = sessions.find((s) => s.id === sessionIdOrAction)
        if (session) {
          route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(session),
          })
        } else {
          route.fulfill({ status: 404 })
        }
      } else if (route.request().method() === 'DELETE') {
        route.fulfill({ status: 204 })
      } else if (route.request().method() === 'PATCH') {
        const session = sessions.find((s) => s.id === sessionIdOrAction)
        if (session) {
          const updates = JSON.parse(route.request().postData() || '{}')
          route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ ...session, ...updates }),
          })
        } else {
          route.fulfill({ status: 404 })
        }
      } else {
        route.continue()
      }
    })

    // Mock session actions (archive, fork)
    await page.route('**/api/sessions/*/archive', (route) => {
      route.fulfill({ status: 200 })
    })

    await page.route('**/api/sessions/*/fork', (route) => {
      if (route.request().method() === 'POST') {
        const postData = JSON.parse(route.request().postData() || '{}')
        const forkedSession = helpers.mockSession({
          id: `forked-${Date.now()}`,
          title: 'Forked Session',
          projectPath: '',
          parentSessionId: 'parent-session',
          forkPointMessageId: postData.forkPointMessageId,
        })

        route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify(forkedSession),
        })
      } else {
        route.continue()
      }
    })

    // Mock messages endpoint
    await page.route('**/api/sessions/*/messages', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      })
    })
  },

  /**
   * Create mock provider with full details
   */
  mockProvider(params: {
    id: string
    name: string
    models?: Array<{
      id: string
      name: string
      capabilities?: {
        streaming?: boolean
        vision?: boolean
        toolUse?: boolean
        systemPrompt?: boolean
        multiTurn?: boolean
        maxContextTokens?: number
        maxOutputTokens?: number
      }
      pricePerMInputTokens?: number
      pricePerMOutputTokens?: number
    }>
    isAvailable?: boolean
  }) {
    return {
      id: params.id,
      name: params.name,
      models: params.models ?? [
        {
          id: `${params.id}-default-model`,
          name: `${params.name} Default Model`,
          capabilities: {
            streaming: true,
            vision: true,
            toolUse: true,
            systemPrompt: true,
            multiTurn: true,
            maxContextTokens: 128000,
            maxOutputTokens: 4096,
          },
          pricePerMInputTokens: 1.0,
          pricePerMOutputTokens: 3.0,
        },
      ],
      isAvailable: params.isAvailable ?? true,
    }
  },

  /**
   * Create mock model info
   */
  mockModelInfo(params: {
    id: string
    name: string
    provider: string
    capabilities?: {
      streaming?: boolean
      vision?: boolean
      toolUse?: boolean
      systemPrompt?: boolean
      multiTurn?: boolean
      maxContextTokens?: number
      maxOutputTokens?: number
    }
    pricePerMInputTokens?: number
    pricePerMOutputTokens?: number
  }) {
    return {
      id: params.id,
      name: params.name,
      provider: params.provider,
      capabilities: params.capabilities ?? {
        streaming: true,
        vision: true,
        toolUse: true,
        systemPrompt: true,
        multiTurn: true,
        maxContextTokens: 128000,
        maxOutputTokens: 4096,
      },
      pricePerMInputTokens: params.pricePerMInputTokens ?? 1.0,
      pricePerMOutputTokens: params.pricePerMOutputTokens ?? 3.0,
    }
  },

  /**
   * Create mock loop event
   */
  mockLoopEvent(params: {
    type: string
    sessionId: string
    iteration?: number
    message?: string
    usage?: {
      promptTokens: number
      completionTokens: number
      totalTokens: number
    }
    error?: {
      code: string
      message: string
    }
  }) {
    return {
      type: params.type,
      sessionId: params.sessionId,
      iteration: params.iteration ?? 1,
      message: params.message,
      usage: params.usage,
      error: params.error,
      timestamp: new Date().toISOString(),
    }
  },

  /**
   * Create mock tool call
   */
  mockToolCall(params: {
    toolUseId: string
    toolName: string
    toolInput: Record<string, unknown>
  }) {
    return {
      type: 'tool_use',
      toolUseId: params.toolUseId,
      toolName: params.toolName,
      toolInput: params.toolInput,
    }
  },

  /**
   * Create mock tool result
   */
  mockToolResult(params: {
    toolResultId: string
    output: string
    isError?: boolean
  }) {
    return {
      type: 'tool_result',
      toolResultId: params.toolResultId,
      toolOutput: params.output,
      toolError: params.isError ? params.output : undefined,
    }
  },

  /**
   * Setup comprehensive provider mocking for provider-agnostic loop tests
   */
  async setupProviderMocks(
    page: Page,
    options: {
      providers?: ReturnType<typeof helpers.mockProviders>
      availableProviders?: string[]
      unavailableProviders?: string[]
    } = {}
  ) {
    const allProviders = options.providers ?? helpers.mockProviders()
    const availableProviders = options.availableProviders
    const unavailableProviders = options.unavailableProviders ?? []

    // Compute availability for each provider
    const providers = allProviders.map((p) => ({
      ...p,
      isAvailable:
        (availableProviders ? availableProviders.includes(p.id) : p.isAvailable) &&
        !unavailableProviders.includes(p.id),
    }))

    // Mock WebSocket
    await page.route('**/ws', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'text/plain',
        body: '',
      })
    })

    // Mock providers list endpoint
    await page.route('**/api/providers', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(providers),
      })
    })

    // Mock individual provider availability
    await page.route('**/api/providers/*/availability', (route) => {
      const url = route.request().url()
      const providerMatch = url.match(/\/providers\/([^/]+)\/availability/)
      const providerId = providerMatch?.[1]

      const provider = providers.find((p) => p.id === providerId)
      const isAvailable = provider?.isAvailable ?? false

      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          provider: providerId,
          available: isAvailable,
          reason: isAvailable ? 'API key configured' : 'API key not configured',
        }),
      })
    })

    // Mock provider models endpoint
    await page.route('**/api/providers/*/models', (route) => {
      const url = route.request().url()
      const providerMatch = url.match(/\/providers\/([^/]+)\/models/)
      const providerId = providerMatch?.[1]

      const provider = providers.find((p) => p.id === providerId)
      if (provider) {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(provider.models),
        })
      } else {
        route.fulfill({
          status: 404,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'Provider not found' }),
        })
      }
    })

    // Mock model info endpoint
    await page.route('**/api/models/*', (route) => {
      const url = route.request().url()
      const modelId = url.split('/').pop()?.split('?')[0]

      // Find model in any provider
      for (const provider of providers) {
        const model = provider.models.find((m) => m.id === modelId)
        if (model) {
          route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              ...model,
              provider: provider.id,
            }),
          })
          return
        }
      }

      route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Model not found' }),
      })
    })
  },
}
