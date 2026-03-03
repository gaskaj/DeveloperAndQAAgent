package developer

import (
	"context"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockWorkspacePersistence implements workspace persistence for testing
type MockWorkspacePersistence struct {
	mock.Mock
}

func (m *MockWorkspacePersistence) CreateSnapshot(ctx context.Context, workspaceDir string, agentState *state.AgentWorkState) (*workspace.WorkspaceSnapshot, error) {
	args := m.Called(ctx, workspaceDir, agentState)
	return args.Get(0).(*workspace.WorkspaceSnapshot), args.Error(1)
}

func (m *MockWorkspacePersistence) RestoreSnapshot(ctx context.Context, issueNumber int) (*workspace.WorkspaceSnapshot, error) {
	args := m.Called(ctx, issueNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*workspace.WorkspaceSnapshot), args.Error(1)
}

func (m *MockWorkspacePersistence) ValidateSnapshot(snapshot *workspace.WorkspaceSnapshot) error {
	args := m.Called(snapshot)
	return args.Error(0)
}

func (m *MockWorkspacePersistence) GetSnapshots(issueNumber int) ([]*workspace.WorkspaceSnapshot, error) {
	args := m.Called(issueNumber)
	return args.Get(0).([]*workspace.WorkspaceSnapshot), args.Error(1)
}

func (m *MockWorkspacePersistence) DeleteSnapshot(snapshotID string) error {
	args := m.Called(snapshotID)
	return args.Error(0)
}

func TestResumeManager_AnalyzeResumeOptions(t *testing.T) {
	mockPersistence := &MockWorkspacePersistence{}
	mockRecovery := &MockRecoveryManager{}
	
	resumeManager := NewResumeManager(mockPersistence, mockRecovery, nil)

	ctx := context.Background()
	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	tests := []struct {
		name             string
		setupMock        func()
		expectedStrategy string
		expectError      bool
	}{
		{
			name: "No snapshots available",
			setupMock: func() {
				mockPersistence.On("RestoreSnapshot", ctx, 123).Return(nil, nil)
			},
			expectedStrategy: "clean_start",
			expectError:      false,
		},
		{
			name: "Valid snapshot available",
			setupMock: func() {
				snapshot := &workspace.WorkspaceSnapshot{
					ID:          "test-snapshot",
					IssueNumber: 123,
					Timestamp:   time.Now().Add(-30 * time.Minute), // 30 minutes old
					AgentState: &state.AgentWorkState{
						State: state.StateImplement,
					},
					FileStates: map[string]workspace.FileState{
						"test.txt": {
							Path: "test.txt",
							Hash: "abc123",
						},
					},
					ImplementationHash: "hash123",
				}
				mockPersistence.On("RestoreSnapshot", ctx, 123).Return(snapshot, nil)
			},
			expectedStrategy: "implement",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockPersistence.ExpectedCalls = nil
			mockRecovery.ExpectedCalls = nil

			tt.setupMock()

			resumePoint, err := resumeManager.AnalyzeResumeOptions(ctx, agentState)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resumePoint)
				assert.Equal(t, tt.expectedStrategy, resumePoint.Stage)
				assert.NotNil(t, resumePoint.RecommendedActions)
				assert.NotEmpty(t, resumePoint.ConfidenceLevel)
				assert.NotEmpty(t, resumePoint.RiskAssessment)
			}

			mockPersistence.AssertExpectations(t)
		})
	}
}

func TestResumeManager_DetermineOptimalResumeStrategy(t *testing.T) {
	resumeManager := NewResumeManager(nil, nil, nil)

	agentState := &state.AgentWorkState{
		State: state.StateImplement,
	}

	tests := []struct {
		name             string
		snapshot         *workspace.WorkspaceSnapshot
		expectedStrategy ResumeStrategy
	}{
		{
			name:             "No snapshot",
			snapshot:         nil,
			expectedStrategy: ResumeStrategyCleanRestart,
		},
		{
			name: "Snapshot too old",
			snapshot: &workspace.WorkspaceSnapshot{
				Timestamp:  time.Now().Add(-25 * time.Hour), // Older than 24 hours
				AgentState: &state.AgentWorkState{State: state.StateImplement},
			},
			expectedStrategy: ResumeStrategyCleanRestart,
		},
		{
			name: "Incompatible states",
			snapshot: &workspace.WorkspaceSnapshot{
				Timestamp:  time.Now().Add(-1 * time.Hour),
				AgentState: &state.AgentWorkState{State: state.StateAnalyze}, // Different from current
			},
			expectedStrategy: ResumeStrategyFromCheckpoint,
		},
		{
			name: "Valid snapshot",
			snapshot: &workspace.WorkspaceSnapshot{
				Timestamp:  time.Now().Add(-1 * time.Hour),
				AgentState: &state.AgentWorkState{State: state.StateImplement}, // Same as current
			},
			expectedStrategy: ResumeStrategyFromSnapshot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := resumeManager.DetermineOptimalResumeStrategy(context.Background(), agentState, tt.snapshot)
			assert.Equal(t, tt.expectedStrategy, strategy)
		})
	}
}

func TestResumeManager_CalculateResumeSavings(t *testing.T) {
	resumeManager := NewResumeManager(nil, nil, nil)

	tests := []struct {
		name           string
		resumePoint    *ResumePoint
		expectSavings  bool
		expectedMinimum time.Duration
	}{
		{
			name: "No snapshot",
			resumePoint: &ResumePoint{
				Snapshot: nil,
			},
			expectSavings: false,
		},
		{
			name: "Implement stage with high confidence",
			resumePoint: &ResumePoint{
				Snapshot: &workspace.WorkspaceSnapshot{
					AgentState: &state.AgentWorkState{State: state.StateImplement},
				},
				ConfidenceLevel: ResumeConfidenceHigh,
				RiskAssessment:  ResumeRiskLow,
			},
			expectSavings:   true,
			expectedMinimum: 10 * time.Minute, // Base savings for implement stage
		},
		{
			name: "Analyze stage with medium confidence",
			resumePoint: &ResumePoint{
				Snapshot: &workspace.WorkspaceSnapshot{
					AgentState: &state.AgentWorkState{State: state.StateAnalyze},
				},
				ConfidenceLevel: ResumeConfidenceMedium,
				RiskAssessment:  ResumeRiskMedium,
			},
			expectSavings:   true,
			expectedMinimum: 5 * time.Minute, // Reduced due to confidence/risk factors
		},
		{
			name: "Unknown stage",
			resumePoint: &ResumePoint{
				Snapshot: &workspace.WorkspaceSnapshot{
					AgentState: &state.AgentWorkState{State: "unknown"},
				},
				ConfidenceLevel: ResumeConfidenceHigh,
				RiskAssessment:  ResumeRiskLow,
			},
			expectSavings: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			savings := resumeManager.CalculateResumeSavings(tt.resumePoint)
			
			if tt.expectSavings {
				assert.True(t, savings > 0, "Expected positive savings")
				if tt.expectedMinimum > 0 {
					// Allow for some flexibility due to confidence/risk adjustments
					assert.True(t, savings >= tt.expectedMinimum/2, 
						"Savings should be at least half the expected minimum after adjustments")
				}
			} else {
				assert.Equal(t, time.Duration(0), savings)
			}
		})
	}
}

func TestResumeManager_AssessResumeConfidence(t *testing.T) {
	resumeManager := NewResumeManager(nil, nil, nil)

	agentState := &state.AgentWorkState{State: state.StateImplement}

	tests := []struct {
		name               string
		snapshot           *workspace.WorkspaceSnapshot
		expectedConfidence ResumeConfidence
	}{
		{
			name: "Recent snapshot with compatible state",
			snapshot: &workspace.WorkspaceSnapshot{
				Timestamp:  time.Now().Add(-10 * time.Minute), // Recent
				AgentState: &state.AgentWorkState{State: state.StateImplement}, // Compatible
				FileStates: map[string]workspace.FileState{
					"test.txt": {Path: "test.txt"},
				}, // Small number of files
			},
			expectedConfidence: ResumeConfidenceHigh,
		},
		{
			name: "Old snapshot with incompatible state",
			snapshot: &workspace.WorkspaceSnapshot{
				Timestamp:  time.Now().Add(-3 * time.Hour), // Old
				AgentState: &state.AgentWorkState{State: state.StateAnalyze}, // Incompatible
				FileStates: make(map[string]workspace.FileState), // Many files would be created here in real test
			},
			expectedConfidence: ResumeConfidenceLow,
		},
		{
			name: "Medium age snapshot",
			snapshot: &workspace.WorkspaceSnapshot{
				Timestamp:  time.Now().Add(-1 * time.Hour), // Medium age
				AgentState: &state.AgentWorkState{State: state.StateImplement}, // Compatible
				FileStates: map[string]workspace.FileState{
					"test.txt": {Path: "test.txt"},
				},
			},
			expectedConfidence: ResumeConfidenceHigh, // Still should be high due to compatibility
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := resumeManager.assessResumeConfidence(agentState, tt.snapshot)
			assert.Equal(t, tt.expectedConfidence, confidence)
		})
	}
}

func TestResumeManager_AssessResumeRisk(t *testing.T) {
	resumeManager := NewResumeManager(nil, nil, nil)

	tests := []struct {
		name         string
		snapshot     *workspace.WorkspaceSnapshot
		expectedRisk ResumeRisk
	}{
		{
			name: "Early stage with no changes",
			snapshot: &workspace.WorkspaceSnapshot{
				AgentState: &state.AgentWorkState{State: state.StateAnalyze},
				FileStates: map[string]workspace.FileState{
					"test.txt": {Path: "test.txt", IsModified: false},
				},
				GitState: workspace.GitSnapshot{HasChanges: false},
			},
			expectedRisk: ResumeRiskLow,
		},
		{
			name: "Commit stage with changes",
			snapshot: &workspace.WorkspaceSnapshot{
				AgentState: &state.AgentWorkState{State: state.StateCommit},
				FileStates: map[string]workspace.FileState{
					"test1.txt": {Path: "test1.txt", IsModified: true},
					"test2.txt": {Path: "test2.txt", IsModified: true},
					"test3.txt": {Path: "test3.txt", IsModified: true},
				},
				GitState: workspace.GitSnapshot{HasChanges: true},
			},
			expectedRisk: ResumeRiskHigh, // High risk due to commit stage + changes
		},
		{
			name: "Implement stage with moderate changes",
			snapshot: &workspace.WorkspaceSnapshot{
				AgentState: &state.AgentWorkState{State: state.StateImplement},
				FileStates: map[string]workspace.FileState{
					"test1.txt": {Path: "test1.txt", IsModified: true},
					"test2.txt": {Path: "test2.txt", IsModified: false},
				},
				GitState: workspace.GitSnapshot{HasChanges: false},
			},
			expectedRisk: ResumeRiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := resumeManager.assessResumeRisk(tt.snapshot)
			assert.Equal(t, tt.expectedRisk, risk)
		})
	}
}

func TestResumeManager_IsStateCompatible(t *testing.T) {
	resumeManager := NewResumeManager(nil, nil, nil)

	tests := []struct {
		name           string
		currentState   state.WorkflowState
		snapshotState  state.WorkflowState
		expectedResult bool
	}{
		{
			name:           "Same states",
			currentState:   state.StateImplement,
			snapshotState:  state.StateImplement,
			expectedResult: true,
		},
		{
			name:           "Compatible forward transition",
			currentState:   state.StateWorkspace,
			snapshotState:  state.StateAnalyze,
			expectedResult: true, // Can move from analyze to workspace
		},
		{
			name:           "Incompatible transition",
			currentState:   state.StateAnalyze,
			snapshotState:  state.StateImplement,
			expectedResult: false, // Can't go back from implement to analyze
		},
		{
			name:           "Valid progression",
			currentState:   state.StateCommit,
			snapshotState:  state.StateImplement,
			expectedResult: true, // Can move from implement to commit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resumeManager.isStateCompatible(tt.currentState, tt.snapshotState)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestResumeManager_ExecuteResume(t *testing.T) {
	resumeManager := NewResumeManager(nil, nil, nil)

	resumePoint := &ResumePoint{
		Stage: "test",
		RecommendedActions: []ResumeAction{
			ResumeActionValidateFiles,
			ResumeActionVerifyBuild,
		},
		ConfidenceLevel: ResumeConfidenceHigh,
		Snapshot: &workspace.WorkspaceSnapshot{
			IssueNumber: 123,
		},
	}

	ctx := context.Background()
	workspaceDir := "/tmp/test-workspace"

	// Execute resume (should not error for the simplified test actions)
	err := resumeManager.ExecuteResume(ctx, resumePoint, workspaceDir)
	require.NoError(t, err)
}

// MockRecoveryManager for testing
type MockRecoveryManager struct {
	mock.Mock
}

func (m *MockRecoveryManager) AttemptResume(ctx context.Context, workState *state.AgentWorkState) (*ResumptionPlan, error) {
	args := m.Called(ctx, workState)
	return args.Get(0).(*ResumptionPlan), args.Error(1)
}