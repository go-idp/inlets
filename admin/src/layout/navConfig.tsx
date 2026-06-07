import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  Globe,
  Info,
  LayoutDashboard,
  List,
  ScrollText,
  Settings,
} from 'lucide-react'

export type NavItem = {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
}

export type NavGroup = {
  label: string
  items: NavItem[]
}

export const navGroups: NavGroup[] = [
  {
    label: '监控',
    items: [
      { to: '/', label: '运行概览', icon: LayoutDashboard, end: true },
      { to: '/sessions', label: '在线会话', icon: Activity },
      { to: '/domains', label: '子域映射', icon: Globe },
      { to: '/stats', label: '流量统计', icon: List },
    ],
  },
  {
    label: '管理',
    items: [
      { to: '/config', label: '配置管理', icon: Settings },
      { to: '/audit', label: '操作审计', icon: ScrollText },
    ],
  },
  {
    label: '系统',
    items: [{ to: '/status', label: '服务信息', icon: Info }],
  },
]
