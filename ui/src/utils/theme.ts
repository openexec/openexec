/**
 * Centralized theme constants for consistent styling across components.
 * @module utils/theme
 */

/**
 * Color palette - GitHub-inspired dark theme colors
 */
export const colors = {
  // Backgrounds
  bg: {
    primary: '#0d1117',
    secondary: '#161b22',
    tertiary: '#21262d',
    border: '#30363d',
    borderSubtle: '#21262d',
  },

  // Text
  text: {
    primary: '#c9d1d9',
    secondary: '#8b949e',
    muted: '#6e7681',
  },

  // Status colors
  status: {
    success: '#238636',
    successText: '#3fb950',
    warning: '#f0883e',
    error: '#da3633',
    errorText: '#f85149',
    info: '#58a6ff',
    neutral: '#8b949e',
  },

  // Risk levels
  risk: {
    low: '#238636',
    medium: '#f0883e',
    high: '#da3633',
  },
} as const

/**
 * Typography constants
 */
export const typography = {
  fontFamily: {
    mono: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    sans: '-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif',
  },
  fontSize: {
    xs: '10px',
    sm: '11px',
    base: '12px',
    md: '13px',
    lg: '14px',
  },
} as const

/**
 * Spacing constants
 */
export const spacing = {
  xs: '4px',
  sm: '6px',
  md: '8px',
  lg: '12px',
  xl: '16px',
} as const

/**
 * Border radius constants
 */
export const borderRadius = {
  sm: '3px',
  md: '4px',
  lg: '6px',
  xl: '8px',
} as const

/**
 * Get risk level color
 */
export const getRiskColor = (riskLevel?: 'low' | 'medium' | 'high'): string => {
  switch (riskLevel) {
    case 'high':
      return colors.risk.high
    case 'medium':
      return colors.risk.medium
    case 'low':
    default:
      return colors.risk.low
  }
}

/**
 * Tool call status configuration
 */
export interface StatusInfo {
  color: string
  label: string
}

export const statusConfig: Record<string, StatusInfo> = {
  pending: { color: colors.status.warning, label: 'Pending' },
  running: { color: colors.status.info, label: 'Running' },
  completed: { color: colors.status.success, label: 'Completed' },
  failed: { color: colors.status.error, label: 'Failed' },
  cancelled: { color: colors.status.neutral, label: 'Cancelled' },
  timeout: { color: colors.status.error, label: 'Timeout' },
}

/**
 * Get status info for a tool call status
 */
export const getStatusInfo = (status: string): StatusInfo => {
  return statusConfig[status] ?? { color: colors.status.neutral, label: 'Unknown' }
}
