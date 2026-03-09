import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Bot,
  Activity,
  Settings,
  Terminal,
  Key,
  Wrench,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/store/auth-store';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/agents', label: 'Agents', icon: Bot },
  { to: '/config', label: 'Configuration', icon: Wrench },
  { to: '/events', label: 'Events', icon: Activity },
  { to: '/settings', label: 'Settings', icon: Settings },
  { to: '/settings/api-keys', label: 'API Keys', icon: Key },
  { to: '/audit', label: 'Agent Logs', icon: Terminal },
];

export function Sidebar() {
  const currentOrg = useAuthStore((s) => s.currentOrg);

  return (
    <aside className="flex w-[260px] flex-col border-r border-zinc-700 bg-zinc-900">
      {/* Logo / App name */}
      <div className="flex h-16 items-center gap-2 border-b border-zinc-700 px-5">
        <Bot className="h-7 w-7 text-blue-500" />
        <span className="text-lg font-semibold text-zinc-100">
          OpenAgent
        </span>
      </div>

      {/* Org name */}
      {currentOrg && (
        <div className="border-b border-zinc-800 px-5 py-3">
          <p className="text-xs font-medium uppercase tracking-wider text-zinc-500">
            Organization
          </p>
          <p className="mt-0.5 truncate text-sm text-zinc-300">
            {currentOrg.name}
          </p>
        </div>
      )}

      {/* Nav links */}
      <nav className="flex-1 space-y-1 px-3 py-4">
        {navItems.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/settings'}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-blue-600/20 text-blue-400'
                  : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200',
              )
            }
          >
            <Icon className="h-5 w-5" />
            {label}
          </NavLink>
        ))}
      </nav>

      {/* Footer */}
      <div className="border-t border-zinc-800 px-5 py-3">
        <p className="text-xs text-zinc-600">OAF Control Plane v0.1.0</p>
      </div>
    </aside>
  );
}
