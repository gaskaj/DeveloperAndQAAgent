import { useEffect, useState } from 'react';
import { Settings, Save, RotateCcw, ChevronDown, ChevronRight, RotateCw } from 'lucide-react';
import { useAuthStore } from '@/store/auth-store';
import { useConfigStore } from '@/store/config-store';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

const AGENT_TYPES = ['developer'];

interface ConfigField {
  key: string;
  label: string;
  type: string;
  placeholder?: string;
  options?: string[];
  description?: string;
}

interface ConfigSection {
  key: string;
  label: string;
  description?: string;
  nested?: string;
  fields: ConfigField[];
}

// Global developer config sections — excludes github owner/repo/token (agent-level only) and controlplane settings
const CONFIG_SECTIONS: ConfigSection[] = [
  {
    key: 'github',
    label: 'GitHub',
    description: 'Shared GitHub polling and label settings. Repository-specific settings (owner, repo, token) are configured per-agent.',
    fields: [
      { key: 'poll_interval', label: 'Poll Interval', type: 'text', placeholder: '30s', description: 'How often to check for new issues (e.g. 30s, 1m, 5m)' },
      { key: 'watch_labels', label: 'Watch Labels', type: 'text', placeholder: 'agent:ready', description: 'Comma-separated labels to watch for claimable issues' },
    ],
  },
  {
    key: 'claude',
    label: 'Claude AI',
    description: 'Claude model and token settings for code generation.',
    fields: [
      { key: 'api_key', label: 'API Key', type: 'text', placeholder: 'sk-ant-...', description: 'Anthropic API key (shared default for all agents)' },
      { key: 'model', label: 'Model', type: 'text', placeholder: 'claude-sonnet-4-20250514', description: 'Claude model ID for code generation' },
      { key: 'max_tokens', label: 'Max Tokens', type: 'number', placeholder: '8192', description: 'Maximum tokens per response (1–8192)' },
    ],
  },
  {
    key: 'agents',
    label: 'Agent Settings',
    description: 'Core developer agent behavior.',
    nested: 'developer',
    fields: [
      { key: 'enabled', label: 'Enabled', type: 'boolean', description: 'Enable the developer agent' },
      { key: 'max_concurrent', label: 'Max Concurrent', type: 'number', placeholder: '1', description: 'Maximum concurrent issues to work on simultaneously' },
      { key: 'workspace_dir', label: 'Workspace Directory', type: 'text', placeholder: './workspaces', description: 'Directory for cloned repository workspaces' },
      { key: 'allow_pr_merging', label: 'Allow PR Merging', type: 'boolean', description: 'Auto squash-merge PRs after all checks pass' },
      { key: 'allow_auto_issue_processing', label: 'Auto Issue Processing', type: 'boolean', description: 'Automatically promote suggestions to agent:ready when idle' },
    ],
  },
  {
    key: 'creativity',
    label: 'Creativity Engine',
    description: 'Autonomous improvement suggestions during idle periods.',
    fields: [
      { key: 'enabled', label: 'Enabled', type: 'boolean', description: 'Enable creativity mode when idle' },
      { key: 'idle_threshold_seconds', label: 'Idle Threshold (seconds)', type: 'number', placeholder: '120', description: 'Seconds idle before entering creativity mode' },
      { key: 'suggestion_cooldown_seconds', label: 'Suggestion Cooldown (seconds)', type: 'number', placeholder: '300', description: 'Cooldown between suggestion attempts' },
      { key: 'max_pending_suggestions', label: 'Max Pending Suggestions', type: 'number', placeholder: '5', description: 'Maximum open suggestion issues before pausing' },
      { key: 'max_rejection_history', label: 'Max Rejection History', type: 'number', placeholder: '50', description: 'Maximum rejected titles to remember for dedup' },
    ],
  },
  {
    key: 'decomposition',
    label: 'Issue Decomposition',
    description: 'Automatic breakdown of complex issues into subtasks.',
    fields: [
      { key: 'enabled', label: 'Enabled', type: 'boolean', description: 'Enable automatic issue decomposition' },
      { key: 'max_iteration_budget', label: 'Max Iteration Budget', type: 'number', placeholder: '250', description: 'Issues needing more iterations than this are decomposed' },
      { key: 'max_subtasks', label: 'Max Subtasks', type: 'number', placeholder: '5', description: 'Maximum number of subtasks when decomposing' },
    ],
  },
  {
    key: 'memory',
    label: 'Repository Memory',
    description: 'Persistent learnings to improve Claude efficiency across issues.',
    fields: [
      { key: 'enabled', label: 'Enabled', type: 'boolean', description: 'Enable persistent repository memory' },
      { key: 'max_entries', label: 'Max Entries', type: 'number', placeholder: '100', description: 'Maximum memory entries per repository' },
      { key: 'max_prompt_size', label: 'Max Prompt Size', type: 'number', placeholder: '8000', description: 'Maximum characters of memory injected into prompts' },
      { key: 'extract_on_complete', label: 'Extract on Complete', type: 'boolean', description: 'Extract learnings after successful implementation' },
    ],
  },
  {
    key: 'logging',
    label: 'Logging',
    description: 'Log level, file output, rotation, and cleanup.',
    fields: [
      { key: 'level', label: 'Log Level', type: 'select', options: ['debug', 'info', 'warn', 'error'], description: 'Logging verbosity level' },
      { key: 'file_path', label: 'Log File Path', type: 'text', placeholder: './logs/agent.log', description: 'Log to file instead of stdout (optional)' },
    ],
  },
  {
    key: 'shutdown',
    label: 'Shutdown',
    description: 'Graceful shutdown behavior.',
    fields: [
      { key: 'timeout', label: 'Timeout', type: 'text', placeholder: '30s', description: 'Maximum time to wait for graceful shutdown' },
      { key: 'cleanup_workspaces', label: 'Cleanup Workspaces', type: 'boolean', description: 'Clean up workspace directories on shutdown' },
      { key: 'reset_claims', label: 'Reset Claims', type: 'boolean', description: 'Reset claimed labels to agent:ready on recovery' },
    ],
  },
  {
    key: 'error_handling',
    label: 'Error Handling',
    description: 'Retry policies and circuit breaker settings for external API calls.',
    fields: [
      { key: 'retry_enabled', label: 'Retry Enabled', type: 'boolean', description: 'Enable retry mechanisms for external calls' },
      { key: 'retry_max_attempts', label: 'Retry Max Attempts', type: 'number', placeholder: '3', description: 'Default maximum retry attempts' },
      { key: 'retry_base_delay', label: 'Retry Base Delay', type: 'text', placeholder: '1s', description: 'Initial delay before first retry' },
      { key: 'retry_max_delay', label: 'Retry Max Delay', type: 'text', placeholder: '30s', description: 'Maximum delay between retries' },
      { key: 'retry_backoff_factor', label: 'Retry Backoff Factor', type: 'number', placeholder: '2.0', description: 'Exponential backoff multiplier' },
      { key: 'circuit_breaker_enabled', label: 'Circuit Breaker Enabled', type: 'boolean', description: 'Enable circuit breakers for external dependencies' },
      { key: 'circuit_breaker_max_failures', label: 'CB Max Failures', type: 'number', placeholder: '5', description: 'Failures before opening circuit' },
      { key: 'circuit_breaker_timeout', label: 'CB Timeout', type: 'text', placeholder: '60s', description: 'Time before half-open retry' },
      { key: 'circuit_breaker_failure_ratio', label: 'CB Failure Ratio', type: 'number', placeholder: '0.5', description: 'Failure ratio threshold (0.0-1.0)' },
    ],
  },
];

// Default configuration values matching config.example.yaml (no github owner/repo/token, no controlplane)
const DEFAULT_CONFIG: Record<string, unknown> = {
  github: {
    poll_interval: '30s',
    watch_labels: 'agent:ready',
  },
  claude: {
    model: 'claude-sonnet-4-20250514',
    max_tokens: 8192,
  },
  agents: {
    developer: {
      enabled: true,
      max_concurrent: 1,
      workspace_dir: './workspaces',
      allow_pr_merging: false,
      allow_auto_issue_processing: false,
    },
  },
  creativity: {
    enabled: true,
    idle_threshold_seconds: 120,
    suggestion_cooldown_seconds: 300,
    max_pending_suggestions: 5,
    max_rejection_history: 50,
  },
  decomposition: {
    enabled: true,
    max_iteration_budget: 250,
    max_subtasks: 5,
  },
  memory: {
    enabled: true,
    max_entries: 100,
    max_prompt_size: 8000,
    extract_on_complete: true,
  },
  logging: {
    level: 'info',
    file_path: './logs/agent.log',
  },
  shutdown: {
    timeout: '30s',
    cleanup_workspaces: true,
    reset_claims: true,
  },
  error_handling: {
    retry_enabled: true,
    retry_max_attempts: 3,
    retry_base_delay: '1s',
    retry_max_delay: '30s',
    retry_backoff_factor: 2.0,
    circuit_breaker_enabled: true,
    circuit_breaker_max_failures: 5,
    circuit_breaker_timeout: '60s',
    circuit_breaker_failure_ratio: 0.5,
  },
};

function getNestedValue(obj: Record<string, unknown>, section: ConfigSection, fieldKey: string): unknown {
  const sectionData = obj[section.key] as Record<string, unknown> | undefined;
  if (!sectionData) return undefined;
  if (section.nested) {
    const nested = sectionData[section.nested] as Record<string, unknown> | undefined;
    return nested?.[fieldKey];
  }
  return sectionData[fieldKey];
}

function setNestedValue(
  obj: Record<string, unknown>,
  section: ConfigSection,
  fieldKey: string,
  value: unknown,
): Record<string, unknown> {
  const result = { ...obj };
  if (section.nested) {
    const sectionData = { ...(result[section.key] as Record<string, unknown> || {}) };
    const nestedData = { ...(sectionData[section.nested] as Record<string, unknown> || {}) };
    nestedData[fieldKey] = value;
    sectionData[section.nested] = nestedData;
    result[section.key] = sectionData;
  } else {
    const sectionData = { ...(result[section.key] as Record<string, unknown> || {}) };
    sectionData[fieldKey] = value;
    result[section.key] = sectionData;
  }
  return result;
}

export function ConfigPage() {
  const currentOrg = useAuthStore((s) => s.currentOrg);
  const { selectedTypeConfig, loading, saving, error, fetchTypeConfig, saveTypeConfig, clearError } = useConfigStore();
  const [selectedType, setSelectedType] = useState(AGENT_TYPES[0]);
  const [configData, setConfigData] = useState<Record<string, unknown>>({});
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(CONFIG_SECTIONS.map((s) => s.key)));
  const [jsonMode, setJsonMode] = useState(false);
  const [jsonText, setJsonText] = useState('');
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    if (currentOrg) {
      fetchTypeConfig(currentOrg.slug, selectedType);
    }
  }, [currentOrg, selectedType, fetchTypeConfig]);

  useEffect(() => {
    if (selectedTypeConfig) {
      const cfg = selectedTypeConfig.config ?? {};
      setConfigData(cfg);
      setJsonText(JSON.stringify(cfg, null, 2));
      setHasChanges(false);
    }
  }, [selectedTypeConfig]);

  const toggleSection = (key: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const handleFieldChange = (section: ConfigSection, fieldKey: string, value: unknown) => {
    const updated = setNestedValue(configData, section, fieldKey, value);
    setConfigData(updated);
    setJsonText(JSON.stringify(updated, null, 2));
    setHasChanges(true);
  };

  const handleJsonChange = (text: string) => {
    setJsonText(text);
    try {
      const parsed = JSON.parse(text);
      setConfigData(parsed);
      setHasChanges(true);
    } catch {
      // Invalid JSON, don't update configData
    }
  };

  const handleSave = async () => {
    if (!currentOrg) return;
    let dataToSave = configData;
    if (jsonMode) {
      try {
        dataToSave = JSON.parse(jsonText);
      } catch {
        return;
      }
    }
    await saveTypeConfig(currentOrg.slug, selectedType, dataToSave);
    setHasChanges(false);
  };

  const handleReset = () => {
    if (selectedTypeConfig) {
      const cfg = selectedTypeConfig.config ?? {};
      setConfigData(cfg);
      setJsonText(JSON.stringify(cfg, null, 2));
      setHasChanges(false);
    }
  };

  const handleLoadDefaults = () => {
    const defaults = JSON.parse(JSON.stringify(DEFAULT_CONFIG));
    setConfigData(defaults);
    setJsonText(JSON.stringify(defaults, null, 2));
    setHasChanges(true);
  };

  const isEmptyConfig = Object.keys(configData).length === 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Settings className="h-6 w-6 text-blue-400" />
          <div>
            <h1 className="text-xl font-bold text-zinc-100">Global Agent Configuration</h1>
            <p className="text-sm text-zinc-500">
              Shared defaults for all agents of each type. Per-agent overrides (e.g. GitHub repo) are set on individual agents.
            </p>
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between">
        {/* Agent type tabs */}
        <div className="flex gap-1 border-b border-zinc-700">
          {AGENT_TYPES.map((type) => (
            <button
              key={type}
              onClick={() => setSelectedType(type)}
              className={`px-4 py-2 text-sm font-medium capitalize transition-colors ${
                selectedType === type
                  ? 'border-b-2 border-blue-500 text-blue-400'
                  : 'text-zinc-400 hover:text-zinc-200'
              }`}
            >
              {type}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          {isEmptyConfig && (
            <button
              onClick={handleLoadDefaults}
              className="flex items-center gap-1.5 rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-1.5 text-sm text-emerald-400 transition-colors hover:bg-emerald-500/20"
            >
              <RotateCw className="h-4 w-4" />
              Load Defaults
            </button>
          )}
          <button
            onClick={() => setJsonMode(!jsonMode)}
            className="rounded-lg border border-zinc-600 px-3 py-1.5 text-sm text-zinc-300 transition-colors hover:bg-zinc-700"
          >
            {jsonMode ? 'Form View' : 'JSON View'}
          </button>
          <button
            onClick={handleReset}
            disabled={!hasChanges}
            className="flex items-center gap-1.5 rounded-lg border border-zinc-600 px-3 py-1.5 text-sm text-zinc-300 transition-colors hover:bg-zinc-700 disabled:opacity-40"
          >
            <RotateCcw className="h-4 w-4" />
            Reset
          </button>
          <button
            onClick={handleSave}
            disabled={!hasChanges || saving}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-500 disabled:opacity-40"
          >
            <Save className="h-4 w-4" />
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-center justify-between rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          <span>{error}</span>
          <button onClick={clearError} className="text-red-400 hover:text-red-300">Dismiss</button>
        </div>
      )}

      {loading ? (
        <LoadingSpinner size="lg" className="py-12" />
      ) : jsonMode ? (
        <div className="rounded-lg border border-zinc-700 bg-zinc-800 p-4">
          <textarea
            value={jsonText}
            onChange={(e) => handleJsonChange(e.target.value)}
            className="h-[600px] w-full resize-none rounded-lg bg-zinc-900 p-4 font-mono text-sm text-zinc-300 focus:outline-none focus:ring-1 focus:ring-blue-500"
            spellCheck={false}
          />
        </div>
      ) : (
        <div className="space-y-2">
          {CONFIG_SECTIONS.map((section) => (
            <div key={section.key} className="rounded-lg border border-zinc-700 bg-zinc-800">
              <button
                onClick={() => toggleSection(section.key)}
                className="flex w-full items-center justify-between px-4 py-3 text-left"
              >
                <div>
                  <span className="text-sm font-medium text-zinc-200">{section.label}</span>
                  {section.description && (
                    <p className="text-xs text-zinc-500 mt-0.5">{section.description}</p>
                  )}
                </div>
                {expandedSections.has(section.key) ? (
                  <ChevronDown className="h-4 w-4 text-zinc-400 shrink-0" />
                ) : (
                  <ChevronRight className="h-4 w-4 text-zinc-400 shrink-0" />
                )}
              </button>
              {expandedSections.has(section.key) && (
                <div className="border-t border-zinc-700 px-4 py-3 space-y-3">
                  {section.fields.map((field) => {
                    const value = getNestedValue(configData, section, field.key);
                    return (
                      <div key={field.key} className="flex items-start justify-between gap-4">
                        <div className="min-w-[240px]">
                          <label className="text-sm text-zinc-300">{field.label}</label>
                          {field.description && (
                            <p className="text-xs text-zinc-500 mt-0.5">{field.description}</p>
                          )}
                        </div>
                        {field.type === 'boolean' ? (
                          <button
                            onClick={() => handleFieldChange(section, field.key, !value)}
                            className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${
                              value ? 'bg-blue-600' : 'bg-zinc-600'
                            }`}
                          >
                            <span
                              className={`absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white transition-transform ${
                                value ? 'translate-x-5' : ''
                              }`}
                            />
                          </button>
                        ) : field.type === 'select' ? (
                          <select
                            value={(value as string) ?? ''}
                            onChange={(e) => handleFieldChange(section, field.key, e.target.value)}
                            className="rounded-lg border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          >
                            <option value="">Default</option>
                            {field.options?.map((opt) => (
                              <option key={opt} value={opt}>{opt}</option>
                            ))}
                          </select>
                        ) : field.type === 'number' ? (
                          <input
                            type="number"
                            step="any"
                            value={value !== undefined && value !== null ? String(value) : ''}
                            onChange={(e) => handleFieldChange(section, field.key, e.target.value ? Number(e.target.value) : undefined)}
                            placeholder={field.placeholder}
                            className="w-48 rounded-lg border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 placeholder:text-zinc-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          />
                        ) : (
                          <input
                            type={field.key.includes('token') || field.key.includes('api_key') ? 'password' : 'text'}
                            value={(value as string) ?? ''}
                            onChange={(e) => handleFieldChange(section, field.key, e.target.value || undefined)}
                            placeholder={field.placeholder}
                            className="w-64 rounded-lg border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 placeholder:text-zinc-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          />
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Version info */}
      {selectedTypeConfig && selectedTypeConfig.version > 0 && (
        <div className="text-xs text-zinc-500">
          Configuration version: {selectedTypeConfig.version}
          {selectedTypeConfig.updated_at && ` | Last updated: ${new Date(selectedTypeConfig.updated_at).toLocaleString()}`}
        </div>
      )}
    </div>
  );
}
