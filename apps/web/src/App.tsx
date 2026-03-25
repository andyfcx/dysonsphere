import { Routes, Route, NavLink, useLocation } from 'react-router-dom'
import Overview from './pages/Overview'
import Hosts from './pages/Hosts'
import Jobs from './pages/Jobs'
import JobDetail from './pages/JobDetail'
import Metrics from './pages/Metrics'
import Alerts from './pages/Alerts'

const navItems = [
  { path: '/', label: 'Overview', icon: '⬡' },
  { path: '/hosts', label: 'Hosts', icon: '⬢' },
  { path: '/jobs', label: 'Jobs', icon: '◷' },
  { path: '/metrics', label: 'Metrics', icon: '◈' },
  { path: '/alerts', label: 'Alerts', icon: '◉' },
]

export default function App() {
  const location = useLocation()

  return (
    <div className="layout">
      <nav className="sidebar">
        <div className="sidebar-logo">
          <span>◈</span> Observer
        </div>
        <div className="nav-group">
          <div className="nav-label">Navigation</div>
          {navItems.map((item) => (
            <NavLink
              key={item.path}
              to={item.path}
              end={item.path === '/'}
              className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
            >
              <span>{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </div>
      </nav>
      <main className="main">
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/hosts" element={<Hosts />} />
          <Route path="/jobs" element={<Jobs />} />
          <Route path="/jobs/:id" element={<JobDetail />} />
          <Route path="/metrics" element={<Metrics />} />
          <Route path="/alerts" element={<Alerts />} />
        </Routes>
      </main>
    </div>
  )
}

