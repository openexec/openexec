/**
 * Centralized formatting utilities for consistent display across components.
 * @module utils/formatters
 */

/**
 * Format duration in milliseconds to human-readable string.
 * @param ms Duration in milliseconds
 * @returns Formatted duration string (e.g., "250ms", "1.50s")
 */
export const formatDuration = (ms?: number): string => {
  if (ms === undefined || ms === null) return ''
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

/**
 * Format cost to USD string with appropriate precision.
 * @param cost Cost in USD
 * @param precision Number of decimal places for small amounts (default: 4)
 * @returns Formatted cost string (e.g., "$0.00", "$1.23", "<$0.0001")
 */
export const formatCost = (cost: number, precision: number = 4): string => {
  if (cost === 0) return '$0.00'
  if (cost < 0.0001) return '<$0.0001'
  if (cost < 0.01) return `$${cost.toFixed(precision)}`
  return `$${cost.toFixed(2)}`
}

/**
 * Format token count with thousands separator.
 * @param tokens Token count
 * @returns Formatted token string (e.g., "1,234", "1.2K", "1.5M")
 */
export const formatTokens = (tokens: number): string => {
  if (tokens < 1000) return tokens.toString()
  if (tokens < 10000) return `${(tokens / 1000).toFixed(1)}K`
  if (tokens < 1000000) return `${Math.round(tokens / 1000)}K`
  return `${(tokens / 1000000).toFixed(1)}M`
}

/**
 * Format file size in bytes to human-readable string.
 * @param bytes File size in bytes
 * @returns Formatted size string (e.g., "512 B", "1.5 KB", "2.3 MB")
 */
export const formatFileSize = (bytes: number): string => {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

/**
 * Format percentage with appropriate decimal places.
 * @param value Value between 0 and 1 (or 0 and 100)
 * @param asDecimal If true, value is treated as decimal (0-1)
 * @returns Formatted percentage string (e.g., "50%", "99.5%")
 */
export const formatPercent = (value: number, asDecimal: boolean = true): string => {
  const percent = asDecimal ? value * 100 : value
  if (percent === 0 || percent === 100) return `${percent}%`
  return `${percent.toFixed(1)}%`
}

/**
 * Truncate string with ellipsis.
 * @param str String to truncate
 * @param maxLength Maximum length before truncation
 * @returns Truncated string with ellipsis if needed
 */
export const truncate = (str: string, maxLength: number): string => {
  if (str.length <= maxLength) return str
  return `${str.slice(0, maxLength - 3)}...`
}

/**
 * Tool name display mapping.
 * Maps internal tool names to user-friendly display names.
 */
export const toolNameDisplayMap: Record<string, string> = {
  read_file: 'Read File',
  write_file: 'Write File',
  run_shell_command: 'Run Command',
  git_apply_patch: 'Apply Patch',
  openexec_signal: 'OpenExec Signal',
}

/**
 * Get display name for a tool.
 * @param toolName Internal tool name
 * @returns User-friendly display name
 */
export const getToolDisplayName = (toolName: string): string => {
  return toolNameDisplayMap[toolName] ?? toolName
}
