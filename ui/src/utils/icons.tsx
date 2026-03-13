/**
 * Centralized icon library for consistent icons across components.
 * @module utils/icons
 */
import React from 'react'

/**
 * Common icon props
 */
export interface IconProps {
  size?: number
  className?: string
  style?: React.CSSProperties
}

const defaultIconProps = {
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
}

/**
 * Status icons
 */
export const PendingIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

export const RunningIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
  </svg>
)

export const CompletedIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <path d="M22 11.08V12a10 10 0 11-5.93-9.14" />
    <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
)

export const ErrorIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <circle cx="12" cy="12" r="10" />
    <line x1="15" y1="9" x2="9" y2="15" />
    <line x1="9" y1="9" x2="15" y2="15" />
  </svg>
)

export const CancelledIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <circle cx="12" cy="12" r="10" />
    <line x1="4.93" y1="4.93" x2="19.07" y2="19.07" />
  </svg>
)

export const TimeoutIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
    <line x1="2" y1="2" x2="22" y2="22" />
  </svg>
)

/**
 * Navigation/Action icons
 */
export const ChevronDownIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <polyline points="6 9 12 15 18 9" />
  </svg>
)

export const ChevronUpIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <polyline points="18 15 12 9 6 15" />
  </svg>
)

export const ExpandIcon: React.FC<IconProps> = ({ size = 12, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <polyline points="15 3 21 3 21 9" />
    <polyline points="9 21 3 21 3 15" />
    <line x1="21" y1="3" x2="14" y2="10" />
    <line x1="3" y1="21" x2="10" y2="14" />
  </svg>
)

export const CollapseIcon: React.FC<IconProps> = ({ size = 12, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <polyline points="4 14 10 14 10 20" />
    <polyline points="20 10 14 10 14 4" />
    <line x1="14" y1="10" x2="21" y2="3" />
    <line x1="3" y1="21" x2="10" y2="14" />
  </svg>
)

/**
 * Info/Utility icons
 */
export const InfoIcon: React.FC<IconProps> = ({ size = 12, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={{ flexShrink: 0, ...style }}
    {...defaultIconProps}
  >
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="16" x2="12" y2="12" />
    <line x1="12" y1="8" x2="12.01" y2="8" />
  </svg>
)

export const WarningIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
    <line x1="12" y1="9" x2="12" y2="13" />
    <line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
)

export const CheckIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <polyline points="20 6 9 17 4 12" />
  </svg>
)

export const CloseIcon: React.FC<IconProps> = ({ size = 14, className, style }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    className={className}
    style={style}
    {...defaultIconProps}
  >
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

/**
 * Map status string to icon component
 */
export const StatusIcons: Record<string, React.FC<IconProps>> = {
  pending: PendingIcon,
  running: RunningIcon,
  completed: CompletedIcon,
  failed: ErrorIcon,
  cancelled: CancelledIcon,
  timeout: TimeoutIcon,
}

/**
 * Render a status icon element for a given status.
 * Use this instead of getStatusIcon when you need a JSX element to avoid
 * the "component created during render" anti-pattern.
 */
export const renderStatusIcon = (status: string, props?: IconProps): React.ReactElement => {
  const Icon = StatusIcons[status] ?? PendingIcon
  return <Icon {...props} />
}
