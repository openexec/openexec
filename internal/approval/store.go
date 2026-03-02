package approval

import (
	"context"
)

// Repository defines the interface for approval data persistence.
// Implementations must be thread-safe and support concurrent access.
type Repository interface {
	// ApprovalRequest operations

	// CreateRequest creates a new approval request.
	CreateRequest(ctx context.Context, request *ApprovalRequest) error

	// GetRequest retrieves an approval request by ID.
	GetRequest(ctx context.Context, id string) (*ApprovalRequest, error)

	// GetRequestByToolCallID retrieves an approval request by tool call ID.
	GetRequestByToolCallID(ctx context.Context, toolCallID string) (*ApprovalRequest, error)

	// UpdateRequest updates an existing approval request.
	UpdateRequest(ctx context.Context, request *ApprovalRequest) error

	// ListPendingRequests lists all pending approval requests for a session.
	ListPendingRequests(ctx context.Context, sessionID string) ([]*ApprovalRequest, error)

	// ListRequestsBySession lists all approval requests for a session.
	ListRequestsBySession(ctx context.Context, sessionID string, opts *ListOptions) ([]*ApprovalRequest, error)

	// ListExpiredRequests lists all pending requests that have passed their expiration time.
	ListExpiredRequests(ctx context.Context) ([]*ApprovalRequest, error)

	// ApprovalPolicy operations

	// CreatePolicy creates a new approval policy.
	CreatePolicy(ctx context.Context, policy *ApprovalPolicy) error

	// GetPolicy retrieves an approval policy by ID.
	GetPolicy(ctx context.Context, id string) (*ApprovalPolicy, error)

	// GetPolicyByName retrieves an approval policy by name.
	GetPolicyByName(ctx context.Context, name string) (*ApprovalPolicy, error)

	// UpdatePolicy updates an existing approval policy.
	UpdatePolicy(ctx context.Context, policy *ApprovalPolicy) error

	// DeletePolicy removes an approval policy.
	DeletePolicy(ctx context.Context, id string) error

	// ListPolicies lists all approval policies.
	ListPolicies(ctx context.Context, opts *ListOptions) ([]*ApprovalPolicy, error)

	// ListActivePolicies lists all active approval policies, ordered by priority.
	ListActivePolicies(ctx context.Context) ([]*ApprovalPolicy, error)

	// GetDefaultPolicy retrieves the default fallback policy.
	GetDefaultPolicy(ctx context.Context) (*ApprovalPolicy, error)

	// GetPolicyForProject retrieves the most applicable policy for a project path.
	GetPolicyForProject(ctx context.Context, projectPath string) (*ApprovalPolicy, error)

	// ApprovalDecision operations

	// CreateDecision records a new approval decision.
	CreateDecision(ctx context.Context, decision *ApprovalDecision) error

	// GetDecision retrieves an approval decision by ID.
	GetDecision(ctx context.Context, id string) (*ApprovalDecision, error)

	// GetDecisionByRequestID retrieves the decision for an approval request.
	GetDecisionByRequestID(ctx context.Context, requestID string) (*ApprovalDecision, error)

	// ListDecisionsBySession lists all decisions for requests in a session.
	ListDecisionsBySession(ctx context.Context, sessionID string, opts *ListOptions) ([]*ApprovalDecision, error)

	// Statistics and aggregations

	// CountPendingRequests returns the count of pending requests for a session.
	CountPendingRequests(ctx context.Context, sessionID string) (int, error)

	// CountRequestsByStatus returns counts grouped by status for a session.
	CountRequestsByStatus(ctx context.Context, sessionID string) (map[RequestStatus]int, error)

	// Resource management

	// Close releases any resources held by the repository.
	Close() error
}

// ListOptions provides pagination and filtering options for list operations.
type ListOptions struct {
	// Limit is the maximum number of results to return (0 = no limit).
	Limit int
	// Offset is the number of results to skip.
	Offset int
	// IncludeInactive includes deactivated policies (for policy lists).
	IncludeInactive bool
	// StatusFilter filters results by status (empty = all statuses).
	StatusFilter []RequestStatus
	// RiskLevelFilter filters results by risk level (empty = all levels).
	RiskLevelFilter []RiskLevel
	// ToolNameFilter filters results by tool name (empty = all tools).
	ToolNameFilter string
	// OrderBy specifies the ordering field.
	OrderBy string
	// OrderDesc specifies descending order.
	OrderDesc bool
}

// DefaultListOptions returns the default list options.
func DefaultListOptions() *ListOptions {
	return &ListOptions{
		Limit:     100,
		Offset:    0,
		OrderBy:   "created_at",
		OrderDesc: true,
	}
}
