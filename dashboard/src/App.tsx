import React, { useState, useEffect } from 'react';
import { 
  Activity, Play, Pause, Layers, AlertCircle, RefreshCw, Cpu, 
  BarChart2, Plus, X, LogOut, CheckCircle, 
  FileText, Lock, Sparkles, Search, Mail, Zap, ShieldCheck, ArrowRight
} from 'lucide-react';
import { 
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, 
  ResponsiveContainer, BarChart, Bar
} from 'recharts';
import { motion, AnimatePresence } from 'framer-motion';
import confetti from 'canvas-confetti';
import { api, API_BASE } from './api/client';
import type { Project, Queue, Job, JobDetails, WorkerInfo, SystemHealth, ThroughputPoint, DLQEntry, DLQTriageResult } from './api/client';
import { CommandPalette } from './components/CommandPalette';
import { JsonViewer } from './components/JsonViewer';

export default function App() {
  const [isLoggedIn, setIsLoggedIn] = useState<boolean>(!!localStorage.getItem('token'));
  const [userRole, setUserRole] = useState<string>(localStorage.getItem('role') || 'admin');
  const [wsConnected, setWsConnected] = useState<boolean>(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isRegisterMode, setIsRegisterMode] = useState(false);
  const [authError, setAuthError] = useState('');

  // Project context
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProject, setSelectedProject] = useState<Project | null>(null);
  const [newProjectName, setNewProjectName] = useState('');
  const [showNewProjModal, setShowNewProjModal] = useState(false);

  // Active Tab
  const [activeTab, setActiveTab] = useState<'queues' | 'explorer' | 'workers' | 'dlq' | 'metrics'>('queues');
  const [showCmdPalette, setShowCmdPalette] = useState(false);

  // Global Hotkey for Command Palette (Cmd+K / Ctrl+K)
  useEffect(() => {
    const handleGlobalKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setShowCmdPalette(prev => !prev);
      }
    };
    window.addEventListener('keydown', handleGlobalKey);
    return () => window.removeEventListener('keydown', handleGlobalKey);
  }, []);

  // WebSocket Live Updates Integration
  useEffect(() => {
    if (!isLoggedIn) return;
    const token = localStorage.getItem('token');
    if (!token) return;

    let ws: WebSocket | null = null;
    let reconnectTimeout: any = null;

    const connectWS = () => {
      try {
        const wsBase = API_BASE.replace(/^http/, 'ws');
        const wsUrl = `${wsBase}/api/ws?token=${encodeURIComponent(token)}`;
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
          setWsConnected(true);
        };

        ws.onmessage = (event) => {
          try {
            const data = JSON.parse(event.data);
            if (data.type === 'job.updated' || data.type === 'worker.heartbeat') {
              refreshData();
            }
          } catch (err) {
            console.error('Error parsing WS message:', err);
          }
        };

        ws.onclose = () => {
          setWsConnected(false);
          reconnectTimeout = setTimeout(connectWS, 3000);
        };

        ws.onerror = () => {
          setWsConnected(false);
        };
      } catch (e) {
        setWsConnected(false);
      }
    };

    connectWS();

    return () => {
      if (ws) ws.close();
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
    };
  }, [isLoggedIn]);

  // DLQ State
  const [dlqEntries, setDlqEntries] = useState<DLQEntry[]>([]);
  const [triageResult, setTriageResult] = useState<DLQTriageResult | null>(null);
  const [isTriaging, setIsTriaging] = useState<boolean>(false);
  const [isReplaying, setIsReplaying] = useState<boolean>(false);

  // Queues State
  const [queues, setQueues] = useState<Queue[]>([]);
  const [showNewQueueModal, setShowNewQueueModal] = useState(false);
  const [newQueueName, setNewQueueName] = useState('');
  const [newQueuePriority, setNewQueuePriority] = useState(0);
  const [newQueueLimit, setNewQueueLimit] = useState(10);

  // Jobs State
  const [selectedQueue, setSelectedQueue] = useState<Queue | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selectedJob, setSelectedJob] = useState<JobDetails | null>(null);
  const [showJobModal, setShowJobModal] = useState(false);
  const [jobFilterStatus, setJobFilterStatus] = useState('');
  const [jobFilterType, setJobFilterType] = useState('');

  // Submit Job State
  const [showSubmitJobModal, setShowSubmitJobModal] = useState(false);
  const [submitJobType, setSubmitJobType] = useState('noop');
  const [submitJobPayload, setSubmitJobPayload] = useState('{}');
  const [submitJobPriority, setSubmitJobPriority] = useState(0);
  const [submitJobDelay, setSubmitJobDelay] = useState(0); // minutes
  const [submitJobCron, setSubmitJobCron] = useState('');
  const [submitJobDependsOn, setSubmitJobDependsOn] = useState('');

  // Workers State
  const [workers, setWorkers] = useState<WorkerInfo[]>([]);

  // Metrics State
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [throughput, setThroughput] = useState<ThroughputPoint[]>([]);

  // Global Poll Ticker
  useEffect(() => {
    if (!isLoggedIn) return;

    // Load projects initially
    loadProjects();

    // Fetch immediately on tab or project switch
    refreshData();

    const interval = setInterval(() => {
      refreshData();
    }, 3000);

    return () => clearInterval(interval);
  }, [isLoggedIn, selectedProject?.id, selectedQueue?.id, activeTab]);

  // Live Auto-refresh running job details modal for execution logs streaming
  useEffect(() => {
    if (!showJobModal || !selectedJob || (selectedJob.status !== 'running' && selectedJob.status !== 'claimed')) {
      return;
    }
    const interval = setInterval(async () => {
      try {
        const updated = await api.getJob(selectedJob.id);
        setSelectedJob(updated);
      } catch (e) {
        console.error('Failed to auto-refresh job logs:', e);
      }
    }, 1500);
    return () => clearInterval(interval);
  }, [showJobModal, selectedJob?.id, selectedJob?.status]);

  // Load queues when project changes
  useEffect(() => {
    if (selectedProject) {
      loadQueues(selectedProject.id);
    }
  }, [selectedProject]);

  // Load jobs when queue changes
  useEffect(() => {
    if (selectedQueue) {
      loadJobs(selectedQueue.id);
    }
  }, [selectedQueue, jobFilterStatus, jobFilterType]);

  const loadProjects = async () => {
    try {
      const projs = await api.listProjects();
      setProjects(projs);
      if (projs.length > 0 && !selectedProject) {
        setSelectedProject(projs[0]);
      }
    } catch (e: any) {
      console.error(e);
    }
  };

  const loadQueues = async (projectId: string) => {
    try {
      const qList = await api.listQueues(projectId);
      const withHealth = await Promise.all(
        qList.map(async (q) => {
          try {
            const h = await api.getQueueHealth(q.id);
            return { ...q, health: h };
          } catch {
            return q;
          }
        })
      );
      setQueues(withHealth);
      if (withHealth.length > 0 && !selectedQueue) {
        setSelectedQueue(withHealth[0]);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const loadJobs = async (queueId: string) => {
    try {
      const jList = await api.listJobs(queueId, {
        status: jobFilterStatus || undefined,
        job_type: jobFilterType || undefined,
      });
      setJobs(jList);
    } catch (e) {
      console.error(e);
    }
  };

  const refreshData = () => {
    if (selectedProject) {
      loadQueues(selectedProject.id);
    }
    if (selectedQueue) {
      loadJobs(selectedQueue.id);
    }
    if (activeTab === 'workers') {
      api.listWorkers().then(setWorkers).catch(console.error);
    }
    if (activeTab === 'dlq') {
      api.listDLQ().then(setDlqEntries).catch(console.error);
    }
    if (activeTab === 'metrics') {
      api.getSystemHealth().then(setHealth).catch(console.error);
      api.getThroughput().then(setThroughput).catch(console.error);
    }
  };

  // Auth Operations
  const handleAuth = async (e: React.FormEvent) => {
    e.preventDefault();
    setAuthError('');
    try {
      let res;
      if (isRegisterMode) {
        res = await api.register(email, password);
      } else {
        res = await api.login(email, password);
      }
      const role = (res as any)?.role || 'admin';
      localStorage.setItem('role', role);
      setUserRole(role);
      setIsLoggedIn(true);
    } catch (e: any) {
      setAuthError(e.message || 'Authentication failed');
    }
  };

  const handleLogout = () => {
    api.logout();
    setIsLoggedIn(false);
    setSelectedProject(null);
    setSelectedQueue(null);
    setProjects([]);
    setQueues([]);
  };

  // Project Operations
  const handleCreateProject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newProjectName) return;
    try {
      const proj = await api.createProject(newProjectName);
      setProjects([proj, ...projects]);
      setSelectedProject(proj);
      setNewProjectName('');
      setShowNewProjModal(false);
    } catch (e: any) {
      alert('Error creating project: ' + e.message);
    }
  };

  // Queue Operations
  const handleCreateQueue = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProject || !newQueueName) return;
    try {
      const q = await api.createQueue(selectedProject.id, newQueueName, newQueuePriority, newQueueLimit);
      setQueues([q, ...queues]);
      setSelectedQueue(q);
      setNewQueueName('');
      setShowNewQueueModal(false);
    } catch (e: any) {
      alert('Error creating queue: ' + e.message);
    }
  };

  const handleToggleQueuePause = async (q: Queue) => {
    try {
      await api.updateQueue(q.id, q.priority, q.concurrency_limit, !q.is_paused);
      if (selectedProject) loadQueues(selectedProject.id);
    } catch (e: any) {
      alert('Error updating queue: ' + e.message);
    }
  };

  const handleRunTriage = async () => {
    setIsTriaging(true);
    try {
      const res = await api.getDLQTriage();
      setTriageResult(res);
    } catch (err: any) {
      alert('Error running AI triage: ' + err.message);
    } finally {
      setIsTriaging(false);
    }
  };

  const handleBulkReplayDLQ = async () => {
    if (!confirm('Replay all dead-letter jobs back into the queue?')) return;
    setIsReplaying(true);
    try {
      const res = await api.replayDLQ(selectedQueue?.id);
      confetti({
        particleCount: 80,
        spread: 70,
        origin: { y: 0.6 },
        colors: ['#06b6d4', '#6366f1', '#10b981']
      });
      alert(`Successfully replayed ${res.replayed_count} dead-letter jobs.`);
      setTriageResult(null);
      refreshData();
    } catch (err: any) {
      alert('Error replaying DLQ: ' + err.message);
    } finally {
      setIsReplaying(false);
    }
  };

  // Job Operations
  const handleSubmitJob = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedQueue) return;
    try {
      let parsedPayload = {};
      try {
        parsedPayload = JSON.parse(submitJobPayload);
      } catch {
        alert('Invalid JSON in payload');
        return;
      }

      let runAtStr: string | undefined = undefined;
      if (submitJobDelay > 0) {
        runAtStr = new Date(Date.now() + submitJobDelay * 60000).toISOString();
      }

      const dependsOnArr = submitJobDependsOn.trim() ? submitJobDependsOn.split(',').map(s => s.trim()) : undefined;

      await api.createJob(
        selectedQueue.id,
        submitJobType,
        parsedPayload,
        submitJobPriority,
        runAtStr,
        submitJobCron || undefined,
        dependsOnArr,
      );

      setShowSubmitJobModal(false);
      setSubmitJobPayload('{}');
      setSubmitJobCron('');
      setSubmitJobDelay(0);
      setSubmitJobDependsOn('');
      confetti({
        particleCount: 40,
        spread: 50,
        origin: { y: 0.8 },
        colors: ['#06b6d4', '#10b981']
      });
      loadJobs(selectedQueue.id);
    } catch (e: any) {
      alert('Error creating job: ' + e.message);
    }
  };

  const handleViewJob = async (job: Job) => {
    try {
      const details = await api.getJob(job.id);
      setSelectedJob(details);
      setShowJobModal(true);
    } catch (e: any) {
      alert('Error loading job details: ' + e.message);
    }
  };

  const handleRetryJob = async (jobId: string) => {
    try {
      await api.retryJob(jobId);
      setShowJobModal(false);
      if (selectedQueue) loadJobs(selectedQueue.id);
    } catch (e: any) {
      alert('Error retrying job: ' + e.message);
    }
  };

  const handleCancelJob = async (jobId: string) => {
    try {
      await api.cancelJob(jobId);
      setShowJobModal(false);
      if (selectedQueue) loadJobs(selectedQueue.id);
    } catch (e: any) {
      alert('Error cancelling job: ' + e.message);
    }
  };

  // Auth Screen (Warm Cream & Obsidian Architectural Split)
  if (!isLoggedIn) {
    return (
      <div className="auth-split-wrapper">
        {/* Left Hero Showcase */}
        <div className="auth-showcase-panel">
          <div className="showcase-brand-header">
            <div className="showcase-logo-icon">⬡</div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.6rem' }}>
                <span className="showcase-brand-name">OBSIDIAN</span>
                <span className="showcase-brand-badge">Postgres Native</span>
              </div>
            </div>
          </div>

          <div className="showcase-content">
            <div className="showcase-eyebrow">
              <span style={{ width: 8, height: 8, borderRadius: '50%', background: '#34d399', display: 'inline-block', boxShadow: '0 0 8px #34d399' }} />
              HIGH-CONCURRENCY DISTRIBUTED QUEUES
            </div>

            <h1 className="showcase-title">
              Mission control for <span>async workloads</span>.
            </h1>

            <p className="showcase-description">
              Lock-free background job queues, DAG dependency graphs, and automated DLQ recovery—powered natively by PostgreSQL 16 without Redis or RabbitMQ dependencies.
            </p>

            {/* Pipeline Architecture Diagram */}
            <div className="pipeline-diagram">
              <div className="pipeline-diagram-header">
                <span>Core Execution Pipeline</span>
                <span style={{ color: '#34d399', display: 'flex', alignItems: 'center', gap: '0.35rem' }}>
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#34d399' }} />
                  Zero-Contention
                </span>
              </div>
              <div className="pipeline-flow">
                <div className="pipeline-node" style={{ borderColor: 'rgba(245, 158, 11, 0.4)' }}>
                  <Layers size={14} color="#f59e0b" />
                  <span>Enqueued</span>
                </div>
                <span className="pipeline-arrow">&rarr;</span>
                <div className="pipeline-node" style={{ borderColor: 'rgba(56, 189, 248, 0.4)' }}>
                  <Cpu size={14} color="#38bdf8" />
                  <span>SKIP LOCKED</span>
                </div>
                <span className="pipeline-arrow">&rarr;</span>
                <div className="pipeline-node" style={{ borderColor: 'rgba(16, 185, 129, 0.4)' }}>
                  <CheckCircle size={14} color="#34d399" />
                  <span>JSONB Result</span>
                </div>
              </div>
            </div>

            <div className="showcase-feature-list">
              <div className="showcase-feature-item">
                <div className="showcase-feature-dot" />
                <span>Zero message broker overhead</span>
              </div>
              <div className="showcase-feature-item">
                <div className="showcase-feature-dot" />
                <span>Cooperative LISTEN/NOTIFY cancellation</span>
              </div>
              <div className="showcase-feature-item">
                <div className="showcase-feature-dot" />
                <span>AI DLQ Triage with Gemini 2.5</span>
              </div>
              <div className="showcase-feature-item">
                <div className="showcase-feature-dot" />
                <span>ACID transaction safety guarantee</span>
              </div>
            </div>
          </div>

          <div className="showcase-footer">
            <span>Obsidian v1.4.0</span>
            <span>&bull;</span>
            <span>PostgreSQL 16 Engine</span>
            <span>&bull;</span>
            <span>Go 1.23 Concurrency Pool</span>
          </div>
        </div>

        {/* Right Form Panel: The Warm Cream & Obsidian Auth Deck */}
        <div className="auth-form-panel">
          <div className="cream-card">
            <div className="cream-card-header">
              <div style={{ display: 'inline-flex', padding: '0.3rem 0.65rem', borderRadius: '6px', background: '#f3efe6', color: '#78716c', fontSize: '0.72rem', fontFamily: 'var(--font-mono)', fontWeight: 700, letterSpacing: '0.04em', textTransform: 'uppercase', marginBottom: '0.75rem' }}>
                Secure Console Access
              </div>
              <h2 className="cream-card-title">
                {isRegisterMode ? 'Deploy Your Fleet' : 'Welcome to Obsidian'}
              </h2>
              <p className="cream-card-subtitle">
                {isRegisterMode 
                  ? 'Create an administrative workspace to start orchestrating queues.'
                  : 'Enter your workspace credentials to access real-time telemetry.'}
              </p>
            </div>

            <div className="cream-tabs">
              <button
                type="button"
                className={`cream-tab-btn ${!isRegisterMode ? 'active' : ''}`}
                onClick={() => {
                  setIsRegisterMode(false);
                  setAuthError('');
                }}
              >
                Sign In
              </button>
              <button
                type="button"
                className={`cream-tab-btn ${isRegisterMode ? 'active' : ''}`}
                onClick={() => {
                  setIsRegisterMode(true);
                  setAuthError('');
                }}
              >
                Create Account
              </button>
            </div>

            <button
              type="button"
              className="cream-demo-pill"
              onClick={() => {
                setEmail('demo@obsidian.local');
                setPassword('demopassword123');
                setIsRegisterMode(false);
                setAuthError('');
              }}
              title="Click to automatically load the pre-configured demo account"
            >
              <Zap size={14} color="#b45309" />
              <span>⚡ One-Click Demo Credentials</span>
            </button>

            <form onSubmit={handleAuth}>
              {authError && (
                <div style={{
                  color: '#b91c1c',
                  background: '#fef2f2',
                  border: '1px solid #fecaca',
                  padding: '0.75rem 1rem',
                  borderRadius: '10px',
                  marginBottom: '1.25rem',
                  fontSize: '0.84rem',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '0.5rem'
                }}>
                  <AlertCircle size={15} color="#dc2626" />
                  <span>{authError}</span>
                </div>
              )}

              <div className="cream-form-group">
                <label>Work Email</label>
                <div className="cream-input-wrapper">
                  <Mail size={16} className="cream-input-icon" />
                  <input 
                    type="email" 
                    className="cream-input" 
                    placeholder="name@company.com"
                    value={email} 
                    onChange={e => setEmail(e.target.value)} 
                    required 
                  />
                </div>
              </div>

              <div className="cream-form-group" style={{ marginBottom: '1.5rem' }}>
                <label>Password</label>
                <div className="cream-input-wrapper">
                  <Lock size={16} className="cream-input-icon" />
                  <input 
                    type="password" 
                    className="cream-input" 
                    placeholder="••••••••••••"
                    value={password} 
                    onChange={e => setPassword(e.target.value)} 
                    required 
                  />
                </div>
              </div>

              <button type="submit" className="cream-submit-btn">
                <span>{isRegisterMode ? 'Create Fleet Account' : 'Authenticate Fleet Console'}</span>
                <ArrowRight size={16} />
              </button>
            </form>

            <div className="cream-footer">
              <ShieldCheck size={14} color="#059669" />
              <span>Protected by PostgreSQL 16 Role-Based Security</span>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app-container">
      {/* Sidebar Navigation */}
      <nav className="sidebar">
        <div className="brand">
          <div className="brand-icon-wrapper">
            <div className="brand-icon">⬡</div>
            <div className="brand-icon-pulse" />
          </div>
          <div className="brand-text">
            <h2>OBSIDIAN</h2>
            <span className="brand-tag">Ops Telemetry</span>
          </div>
        </div>

        {/* Project Selector */}
        <div style={{ marginBottom: '2rem' }}>
          <label style={{ display: 'block', fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary)', textTransform: 'uppercase', marginBottom: '0.5rem' }}>Active Project</label>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <select 
              className="form-control" 
              style={{ padding: '0.5rem', background: 'rgba(0, 0, 0, 0.3)' }}
              value={selectedProject?.id || ''}
              onChange={e => {
                const p = projects.find(x => x.id === e.target.value);
                if (p) setSelectedProject(p);
              }}
            >
              {projects.map(p => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            <button 
              className="btn btn-secondary" 
              style={{ padding: '0.5rem' }} 
              onClick={() => setShowNewProjModal(true)}
              title="New Project"
            >
              <Plus size={16} />
            </button>
          </div>
        </div>

        <ul className="nav-links">
          <li className={`nav-item ${activeTab === 'queues' ? 'active' : ''}`} onClick={() => setActiveTab('queues')}>
            <Layers size={18} /> Queues
          </li>
          <li className={`nav-item ${activeTab === 'explorer' ? 'active' : ''}`} onClick={() => setActiveTab('explorer')}>
            <FileText size={18} /> Job Explorer
          </li>
          <li className={`nav-item ${activeTab === 'workers' ? 'active' : ''}`} onClick={() => setActiveTab('workers')}>
            <Cpu size={18} /> Worker Nodes
          </li>
          <li className={`nav-item ${activeTab === 'dlq' ? 'active' : ''}`} onClick={() => setActiveTab('dlq')}>
            <Lock size={18} /> Dead Letter Queue
            {dlqEntries.length > 0 && (
              <span className="nav-badge nav-badge-alert">{dlqEntries.length}</span>
            )}
          </li>
          <li className={`nav-item ${activeTab === 'metrics' ? 'active' : ''}`} onClick={() => setActiveTab('metrics')}>
            <BarChart2 size={18} /> Observability
          </li>
        </ul>

        <div style={{ marginTop: 'auto' }}>
          {selectedProject && (
            <div style={{ background: 'rgba(255,255,255,0.02)', padding: '0.75rem', borderRadius: '8px', marginBottom: '1rem', border: '1px dashed var(--border-color)' }}>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>API Key:</p>
              <code style={{ fontSize: '0.75rem', color: 'var(--text-primary)', wordBreak: 'break-all' }}>{selectedProject.api_key}</code>
            </div>
          )}
          <button onClick={handleLogout} className="btn btn-danger" style={{ width: '100%' }}>
            <LogOut size={16} /> Logout
          </button>
        </div>
      </nav>

      {/* Main Panel */}
      <main className="main-content">
        <header className="header">
          <div className="header-title">
            <h1>
              {activeTab === 'queues' && 'Queue Configuration'}
              {activeTab === 'explorer' && 'Job Explorer'}
              {activeTab === 'workers' && 'Worker Pool Monitoring'}
              {activeTab === 'dlq' && 'Dead Letter Queue Management'}
              {activeTab === 'metrics' && 'Performance & System Metrics'}
            </h1>
            <p>
              Project: <span style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{selectedProject?.name || 'None'}</span>
            </p>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
            <div className="command-bar-trigger" onClick={() => setShowCmdPalette(true)}>
              <Search size={14} />
              <span>Quick Actions</span>
              <kbd className="kbd-shortcut">⌘K</kbd>
            </div>

            <div className="fleet-pulse-pill">
              <div className="pulse-dot" style={{ background: wsConnected ? '#10b981' : '#f59e0b' }} />
              <span>{wsConnected ? 'FLEET ACTIVE' : 'RECONNECTING'}</span>
            </div>

            <div style={{ background: 'rgba(255,255,255,0.06)', border: '1px solid var(--border-color)', padding: '0.35rem 0.75rem', borderRadius: '20px', fontSize: '0.8rem', fontWeight: 600, textTransform: 'capitalize', display: 'flex', alignItems: 'center', gap: '0.4rem', color: userRole === 'admin' ? '#facc15' : userRole === 'member' ? '#60a5fa' : '#9ca3af' }}>
              <span>{userRole === 'admin' ? '👑' : userRole === 'member' ? '🔧' : '👁'} {userRole}</span>
            </div>

            <button className="btn btn-secondary" onClick={refreshData}>
              <RefreshCw size={16} /> Refresh
            </button>
          </div>
        </header>

        {/* Tab content logic with Framer Motion spring physics */}
        <AnimatePresence mode="wait">
          <motion.div
            key={activeTab}
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
            transition={{ duration: 0.18, ease: "easeOut" }}
          >
            {activeTab === 'queues' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            <div className="card">
              <div className="card-title">
                <span>Active Queues</span>
                <button className="btn btn-primary" onClick={() => setShowNewQueueModal(true)}>
                  <Plus size={16} /> Create Queue
                </button>
              </div>

              <div className="table-container">
                <table>
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Health</th>
                      <th>Priority</th>
                      <th>Limit</th>
                      <th>Job Statistics</th>
                      <th>State</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {queues.length === 0 ? (
                      <tr>
                        <td colSpan={7} style={{ textAlign: 'center', padding: '3rem 1rem' }}>
                          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '0.75rem', color: 'var(--text-secondary)' }}>
                            <Layers size={32} style={{ opacity: 0.3 }} />
                            <span>No queues configured. Create one to begin.</span>
                          </div>
                        </td>
                      </tr>
                    ) : (
                      queues.map(q => (
                        <tr key={q.id}>
                          <td style={{ fontWeight: 500 }}>{q.name}</td>
                          <td>
                            {q.health ? (
                              <span 
                                className="badge" 
                                style={{
                                  backgroundColor: q.health.grade === 'A' ? '#059669' : q.health.grade === 'B' ? '#10b981' : q.health.grade === 'C' ? '#f59e0b' : q.health.grade === 'D' ? '#f97316' : '#ef4444',
                                  color: '#fff',
                                  fontWeight: 600,
                                }}
                                title={`Health Score: ${q.health.score}/100, DLQ: ${(q.health.dlq_rate * 100).toFixed(1)}%, Fail: ${(q.health.fail_rate * 100).toFixed(1)}%`}
                              >
                                Grade {q.health.grade} ({q.health.score})
                              </span>
                            ) : (
                              <span style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>N/A</span>
                            )}
                          </td>
                          <td>{q.priority}</td>
                          <td>{q.concurrency_limit}</td>
                          <td>
                            <div style={{ display: 'flex', gap: '0.35rem', flexWrap: 'wrap', fontSize: '0.75rem' }}>
                              <span className="badge badge-queued">Q: {q.stats?.queued_count || 0}</span>
                              <span className="badge badge-running">R: {q.stats?.running_count || 0}</span>
                              <span className="badge badge-completed">C: {q.stats?.completed_count || 0}</span>
                              <span className="badge badge-failed">F: {q.stats?.failed_count || 0}</span>
                            </div>
                          </td>
                          <td>
                            <select
                              value={q.state || (q.is_paused ? 'paused' : 'active')}
                              onChange={async (e) => {
                                try {
                                  await api.updateQueueState(q.id, e.target.value);
                                  if (selectedProject) loadQueues(selectedProject.id);
                                } catch (err: any) {
                                  alert('Failed to update queue state: ' + err.message);
                                }
                              }}
                              style={{
                                background: 'var(--bg-secondary)',
                                color: 'var(--text-primary)',
                                border: '1px solid var(--border-color)',
                                borderRadius: '4px',
                                padding: '0.2rem 0.4rem',
                                fontSize: '0.75rem',
                                cursor: 'pointer',
                              }}
                            >
                              <option value="active">Active</option>
                              <option value="paused">Paused</option>
                              <option value="draining">Draining</option>
                              <option value="maintenance">Maintenance</option>
                            </select>
                          </td>
                          <td style={{ display: 'flex', gap: '0.5rem' }}>
                            <button 
                              className={`btn ${q.is_paused ? 'btn-primary' : 'btn-danger'}`} 
                              style={{ padding: '0.4rem 0.8rem', fontSize: '0.8rem' }}
                              onClick={() => handleToggleQueuePause(q)}
                            >
                              {q.is_paused ? <Play size={12} /> : <Pause size={12} />}
                              {q.is_paused ? 'Resume' : 'Pause'}
                            </button>
                            <button 
                              className="btn btn-secondary" 
                              style={{ padding: '0.4rem 0.8rem', fontSize: '0.8rem' }}
                              onClick={() => { setSelectedQueue(q); setShowSubmitJobModal(true); }}
                            >
                              <Plus size={12} /> Submit Job
                            </button>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )}

        {activeTab === 'explorer' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            <div style={{ display: 'flex', gap: '1rem', alignItems: 'center' }}>
              <div style={{ flex: 1 }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>Select Queue</label>
                <select 
                  className="form-control"
                  value={selectedQueue?.id || ''}
                  onChange={e => {
                    const q = queues.find(x => x.id === e.target.value);
                    if (q) setSelectedQueue(q);
                  }}
                >
                  {queues.map(q => (
                    <option key={q.id} value={q.id}>{q.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>Filter Status</label>
                <select 
                  className="form-control"
                  value={jobFilterStatus}
                  onChange={e => setJobFilterStatus(e.target.value)}
                >
                  <option value="">All Statuses</option>
                  <option value="queued">Queued</option>
                  <option value="scheduled">Scheduled</option>
                  <option value="claimed">Claimed</option>
                  <option value="running">Running</option>
                  <option value="completed">Completed</option>
                  <option value="failed">Failed</option>
                  <option value="dead_letter">Dead Letter</option>
                  <option value="cancelled">Cancelled</option>
                </select>
              </div>
              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>Job Type</label>
                <input 
                  type="text" 
                  className="form-control" 
                  placeholder="e.g. noop"
                  value={jobFilterType}
                  onChange={e => setJobFilterType(e.target.value)}
                />
              </div>
            </div>

            <div className="card">
              <div className="card-title">Job Executions ({jobs.length})</div>
              <div className="table-container">
                <table>
                  <thead>
                    <tr>
                      <th>Job ID</th>
                      <th>Job Type</th>
                      <th>Priority</th>
                      <th>Attempt</th>
                      <th>Execution Time</th>
                      <th>Status</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {jobs.length === 0 ? (
                      <tr>
                        <td colSpan={7} style={{ textAlign: 'center', padding: '3rem 1rem' }}>
                          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '0.75rem', color: 'var(--text-secondary)' }}>
                            <FileText size={32} style={{ opacity: 0.3 }} />
                            <span>No jobs matched filters.</span>
                          </div>
                        </td>
                      </tr>
                    ) : (
                      jobs.map(j => (
                        <tr key={j.id}>
                          <td style={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>{j.id.slice(0, 8)}...</td>
                          <td style={{ fontWeight: 500 }}>{j.job_type}</td>
                          <td>{j.priority}</td>
                          <td>{j.attempt}</td>
                          <td>{new Date(j.run_at).toLocaleString()}</td>
                          <td>
                            <span className={`badge badge-${j.status}`}>
                              {j.status}
                            </span>
                          </td>
                          <td>
                            <button 
                              className="btn btn-secondary" 
                              style={{ padding: '0.4rem 0.8rem', fontSize: '0.8rem' }}
                              onClick={() => handleViewJob(j)}
                            >
                              Inspect
                            </button>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )}

        {activeTab === 'workers' && (
          <div className="card">
            <div className="card-title">Active Workers ({workers.length})</div>
            <div className="table-container">
              <table>
                <thead>
                  <tr>
                    <th>Worker ID</th>
                    <th>Hostname</th>
                    <th>Started At</th>
                    <th>Last Active</th>
                    <th>Current Running Job</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {workers.length === 0 ? (
                    <tr>
                      <td colSpan={6} style={{ textAlign: 'center', padding: '3rem 1rem' }}>
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '0.75rem', color: 'var(--text-secondary)' }}>
                          <Cpu size={32} style={{ opacity: 0.3 }} />
                          <span>No active workers polling database. Start a worker node.</span>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    workers.map(w => (
                      <tr key={w.id}>
                        <td style={{ fontFamily: 'monospace' }}>{w.id}</td>
                        <td>{w.hostname}</td>
                        <td>{new Date(w.started_at).toLocaleTimeString()}</td>
                        <td>{new Date(w.last_seen_at).toLocaleTimeString()}</td>
                        <td>
                          {w.current_job_type ? (
                            <span className="badge badge-running" style={{ fontFamily: 'monospace' }}>
                              {w.current_job_type} ({w.current_job_id?.slice(0, 8)}...)
                            </span>
                          ) : (
                            <span style={{ color: 'var(--text-secondary)', fontSize: '0.8rem' }}>Idle</span>
                          )}
                        </td>
                        <td>
                          <span className={`badge ${w.status === 'active' ? 'badge-completed' : 'badge-failed'}`}>
                            {w.status}
                          </span>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {activeTab === 'dlq' && (
          <div className="card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem', flexWrap: 'wrap', gap: '0.75rem' }}>
              <div className="card-title" style={{ marginBottom: 0 }}>
                Dead Letter Queue Entries ({dlqEntries.length})
              </div>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <button
                  className="btn btn-primary"
                  onClick={handleRunTriage}
                  disabled={isTriaging || dlqEntries.length === 0}
                  style={{ padding: '0.4rem 0.8rem', fontSize: '0.8rem', display: 'flex', alignItems: 'center', gap: '0.4rem' }}
                >
                  <Sparkles size={14} /> {isTriaging ? 'Analyzing...' : 'AI Fleet Triage'}
                </button>
                <button
                  className="btn btn-secondary"
                  onClick={handleBulkReplayDLQ}
                  disabled={isReplaying || dlqEntries.length === 0}
                  style={{ padding: '0.4rem 0.8rem', fontSize: '0.8rem', display: 'flex', alignItems: 'center', gap: '0.4rem' }}
                >
                  <RefreshCw size={14} /> {isReplaying ? 'Replaying...' : 'Replay All Jobs'}
                </button>
              </div>
            </div>

            {triageResult && (
              <div
                style={{
                  background: 'rgba(99, 102, 241, 0.08)',
                  border: '1px solid rgba(99, 102, 241, 0.3)',
                  borderRadius: '8px',
                  padding: '1rem',
                  marginBottom: '1.25rem',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '0.5rem' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                    <Sparkles size={18} style={{ color: '#818cf8' }} />
                    <span style={{ fontWeight: 600, color: '#c7d2fe' }}>Gemini AI Fleet Diagnosis</span>
                    <span className="badge badge-failed" style={{ fontSize: '0.7rem' }}>
                      Primary Cause: {triageResult.primary_root_cause}
                    </span>
                  </div>
                  <button
                    onClick={() => setTriageResult(null)}
                    style={{ background: 'none', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer' }}
                  >
                    <X size={16} />
                  </button>
                </div>
                <p style={{ margin: '0 0 0.5rem 0', fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                  {triageResult.summary}
                </p>
                {triageResult.suggested_action && (
                  <div style={{ fontSize: '0.8rem', color: '#93c5fd' }}>
                    <strong>Recommended Action:</strong> {triageResult.suggested_action}
                  </div>
                )}
              </div>
            )}

            <div className="table-container">
              <table>
                <thead>
                  <tr>
                    <th>DLQ ID</th>
                    <th>Original Job ID</th>
                    <th>Queue Name</th>
                    <th>Failure Reason</th>
                    <th>Attempts</th>
                    <th>Moved At</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {dlqEntries.length === 0 ? (
                    <tr>
                      <td colSpan={7} style={{ textAlign: 'center', padding: '3rem 1rem' }}>
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '0.75rem', color: 'var(--text-secondary)' }}>
                          <CheckCircle size={32} style={{ opacity: 0.3, color: 'var(--status-completed)' }} />
                          <span>Dead Letter Queue is empty! No permanently failed jobs.</span>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    dlqEntries.map(entry => (
                      <tr key={entry.id}>
                        <td style={{ fontFamily: 'monospace' }}>{entry.id.slice(0, 8)}...</td>
                        <td style={{ fontFamily: 'monospace' }}>{entry.original_job_id.slice(0, 8)}...</td>
                        <td>{entry.queue_name}</td>
                        <td style={{ color: 'var(--status-failed)', maxWidth: '250px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {entry.failure_reason}
                        </td>
                        <td>{entry.attempts_made}</td>
                        <td>{new Date(entry.moved_at).toLocaleString()}</td>
                        <td>
                          <button 
                            className="btn btn-primary" 
                            style={{ padding: '0.3rem 0.6rem', fontSize: '0.75rem' }}
                            onClick={() => handleRetryJob(entry.original_job_id)}
                          >
                            <RefreshCw size={12} /> Retry Job
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {activeTab === 'metrics' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            <div className="metrics-grid">
              <div className="card metric-card">
                <div className="metric-icon" style={{ color: 'var(--accent-blue)' }}><Activity /></div>
                <div className="metric-info">
                  <span className="metric-label">Active Workers</span>
                  <span className="metric-value">{health?.active_workers || 0}</span>
                </div>
              </div>
              <div className="card metric-card">
                <div className="metric-icon" style={{ color: 'var(--status-completed)' }}><CheckCircle /></div>
                <div className="metric-info">
                  <span className="metric-label">Successful runs (24h)</span>
                  <span className="metric-value">{health?.success_count || 0}</span>
                </div>
              </div>
              <div className="card metric-card">
                <div className="metric-icon" style={{ color: 'var(--status-failed)' }}><AlertCircle /></div>
                <div className="metric-info">
                  <span className="metric-label">Failed runs (24h)</span>
                  <span className="metric-value">{health?.failed_count || 0}</span>
                </div>
              </div>
              <div className="card metric-card">
                <div className="metric-icon" style={{ color: 'var(--status-dead)' }}><Lock /></div>
                <div className="metric-info">
                  <span className="metric-label">Dead Letter Queue</span>
                  <span className="metric-value">{health?.dlq_count || 0}</span>
                </div>
              </div>
              <div className="card metric-card">
                <div className="metric-icon" style={{ color: '#8b5cf6' }}><RefreshCw /></div>
                <div className="metric-info">
                  <span className="metric-label">Avg Exec Duration</span>
                  <span className="metric-value">{health?.avg_duration_ms || 0} ms</span>
                </div>
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
              <div className="card">
                <div className="card-title">Throughput Timeline (Last 24 Hours)</div>
                <div style={{ height: 300 }}>
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={throughput}>
                      <defs>
                        <linearGradient id="colorSuccess" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="#10b981" stopOpacity={0.2}/>
                          <stop offset="95%" stopColor="#10b981" stopOpacity={0}/>
                        </linearGradient>
                      </defs>
                      <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
                      <XAxis 
                        dataKey="hour" 
                        tickFormatter={(t: any) => new Date(t).toLocaleTimeString([], { hour: '2-digit' })} 
                        stroke="var(--text-secondary)"
                      />
                      <YAxis stroke="var(--text-secondary)" />
                      <Tooltip 
                        contentStyle={{ background: '#0f172a', borderColor: 'var(--border-color)' }}
                        labelFormatter={(t: any) => new Date(t).toLocaleString()}
                      />
                      <Area type="monotone" dataKey="successes" name="Successes" stroke="#10b981" fillOpacity={1} fill="url(#colorSuccess)" />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              </div>

              <div className="card">
                <div className="card-title">Queued / Active Workloads</div>
                <div style={{ height: 300 }}>
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={health?.queue_sizes || []}>
                      <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
                      <XAxis dataKey="name" stroke="var(--text-secondary)" />
                      <YAxis stroke="var(--text-secondary)" />
                      <Tooltip contentStyle={{ background: '#0f172a', borderColor: 'var(--border-color)' }} />
                      <Bar dataKey="size" name="Jobs" fill="#3b82f6" radius={[4, 4, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>
            </div>
          </div>
        )}
          </motion.div>
        </AnimatePresence>
      </main>

      {/* MODALS */}
      {/* Create Project Modal */}
      {showNewProjModal && (
        <div className="modal-backdrop">
          <div className="modal-content">
            <div className="card-title">
              <span>Create New Project</span>
              <X size={18} style={{ cursor: 'pointer' }} onClick={() => setShowNewProjModal(false)} />
            </div>
            <form onSubmit={handleCreateProject}>
              <div className="form-group">
                <label>Project Name</label>
                <input 
                  type="text" 
                  className="form-control" 
                  value={newProjectName} 
                  onChange={e => setNewProjectName(e.target.value)} 
                  required 
                />
              </div>
              <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem', justifyContent: 'flex-end' }}>
                <button type="button" className="btn btn-secondary" onClick={() => setShowNewProjModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Create</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Create Queue Modal */}
      {showNewQueueModal && (
        <div className="modal-backdrop">
          <div className="modal-content">
            <div className="card-title">
              <span>Create New Queue</span>
              <X size={18} style={{ cursor: 'pointer' }} onClick={() => setShowNewQueueModal(false)} />
            </div>
            <form onSubmit={handleCreateQueue}>
              <div className="form-group">
                <label>Queue Name</label>
                <input 
                  type="text" 
                  className="form-control" 
                  value={newQueueName} 
                  onChange={e => setNewQueueName(e.target.value)} 
                  required 
                />
              </div>
              <div className="form-group">
                <label>Priority (Default 0)</label>
                <input 
                  type="number" 
                  className="form-control" 
                  value={newQueuePriority} 
                  onChange={e => setNewQueuePriority(parseInt(e.target.value) || 0)} 
                />
              </div>
              <div className="form-group">
                <label>Max Concurrency Limit</label>
                <input 
                  type="number" 
                  className="form-control" 
                  value={newQueueLimit} 
                  onChange={e => setNewQueueLimit(parseInt(e.target.value) || 10)} 
                />
              </div>
              <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem', justifyContent: 'flex-end' }}>
                <button type="button" className="btn btn-secondary" onClick={() => setShowNewQueueModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Create</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Submit Job Modal */}
      {showSubmitJobModal && (
        <div className="modal-backdrop">
          <div className="modal-content" style={{ maxWidth: '650px' }}>
            <div className="card-title">
              <span>Submit Job to {selectedQueue?.name}</span>
              <X size={18} style={{ cursor: 'pointer' }} onClick={() => setShowSubmitJobModal(false)} />
            </div>
            <form onSubmit={handleSubmitJob}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
                <div className="form-group">
                  <label>Job Type Handler</label>
                  <select 
                    className="form-control" 
                    value={submitJobType} 
                    onChange={e => {
                      const t = e.target.value;
                      setSubmitJobType(t);
                      if (t === 'compute') {
                        setSubmitJobPayload(JSON.stringify({ a: 42, b: 8 }, null, 2));
                      } else if (t === 'log') {
                        setSubmitJobPayload(JSON.stringify({ message: 'Ops telemetry ping' }, null, 2));
                      } else if (t === 'sleep') {
                        setSubmitJobPayload(JSON.stringify({ seconds: 3 }, null, 2));
                      } else if (t === 'fail') {
                        setSubmitJobPayload(JSON.stringify({ reason: 'Simulated exception' }, null, 2));
                      }
                    }}
                  >
                    <option value="noop">noop (instant success)</option>
                    <option value="compute">compute (structured math output)</option>
                    <option value="log">log (console logger)</option>
                    <option value="sleep">sleep (custom timer)</option>
                    <option value="fail">fail (simulates failure)</option>
                  </select>
                </div>
                <div className="form-group">
                  <label>Priority Override (0 = default)</label>
                  <input type="number" className="form-control" value={submitJobPriority} onChange={e => setSubmitJobPriority(parseInt(e.target.value) || 0)} />
                </div>
              </div>
              <div className="form-group">
                <label>JSON Payload Parameters</label>
                <textarea 
                  className="form-control" 
                  style={{ fontFamily: 'monospace', height: '80px', resize: 'vertical' }}
                  value={submitJobPayload} 
                  onChange={e => setSubmitJobPayload(e.target.value)} 
                />
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
                <div className="form-group">
                  <label>Delay Dispatch (Minutes, 0 = immediate)</label>
                  <input type="number" className="form-control" value={submitJobDelay} onChange={e => setSubmitJobDelay(parseInt(e.target.value) || 0)} />
                </div>
                <div className="form-group">
                  <label>Cron Expression (Optional)</label>
                  <input type="text" className="form-control" placeholder="*/5 * * * *" value={submitJobCron} onChange={e => setSubmitJobCron(e.target.value)} />
                </div>
              </div>
              <div className="form-group">
                <label>Depends On Parent Job IDs (Comma separated UUIDs)</label>
                <input type="text" className="form-control" placeholder="UUID_1, UUID_2" value={submitJobDependsOn} onChange={e => setSubmitJobDependsOn(e.target.value)} />
              </div>
              <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem', justifyContent: 'flex-end' }}>
                <button type="button" className="btn btn-secondary" onClick={() => setShowSubmitJobModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Dispatch Job</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Inspect Job Modal */}
      {showJobModal && selectedJob && (
        <div className="modal-backdrop">
          <div className="modal-content" style={{ maxWidth: '750px', maxHeight: '90vh', overflowY: 'auto' }}>
            <div className="card-title">
              <span>Inspect Job: {selectedJob.id}</span>
              <X size={18} style={{ cursor: 'pointer' }} onClick={() => setShowJobModal(false)} />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem', marginBottom: '1.5rem' }}>
              <div>
                <p style={{ color: 'var(--text-secondary)', fontSize: '0.8rem' }}>Job Handler Type</p>
                <p style={{ fontWeight: 500, fontSize: '1rem' }}>{selectedJob.job_type}</p>
              </div>
              <div>
                <p style={{ color: 'var(--text-secondary)', fontSize: '0.8rem' }}>Current Status</p>
                <span className={`badge badge-${selectedJob.status}`} style={{ marginTop: '0.25rem' }}>{selectedJob.status}</span>
              </div>
            </div>

            {selectedJob.ai_summary && (
              <div style={{ background: 'linear-gradient(135deg, rgba(147,51,234,0.15), rgba(79,70,229,0.15))', border: '1px solid rgba(168,85,247,0.35)', borderRadius: '8px', padding: '1rem', marginBottom: '1.5rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontWeight: 600, color: '#c084fc', fontSize: '0.875rem', marginBottom: '0.35rem' }}>
                  <span>🤖 AI Failure Diagnosis & Root Cause</span>
                </div>
                <p style={{ fontSize: '0.85rem', color: '#e9d5ff', margin: 0, lineHeight: '1.4' }}>
                  {selectedJob.ai_summary}
                </p>
              </div>
            )}

            <div className="form-group">
              <JsonViewer data={selectedJob.payload} title="ARGUMENTS (INPUT PAYLOAD)" />
            </div>

            {selectedJob.result && (
              <div className="form-group" style={{ marginTop: '1rem' }}>
                <JsonViewer data={selectedJob.result} title="STRUCTURED OUTPUT (RESULT JSONB)" />
              </div>
            )}

            <div style={{ marginBottom: '1.5rem', marginTop: '1.5rem' }}>
              <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', fontWeight: 500, marginBottom: '0.5rem' }}>Execution Timeline</label>
              {selectedJob.executions.length === 0 ? (
                <p style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>No runs recorded yet.</p>
              ) : (
                <div style={{ background: 'rgba(0,0,0,0.2)', padding: '0.5rem', borderRadius: '8px', display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                  {selectedJob.executions.map((ex, i) => (
                    <div key={ex.id} style={{ display: 'flex', justifyContent: 'space-between', padding: '0.5rem', borderBottom: i < selectedJob.executions.length - 1 ? '1px solid var(--border-color)' : 'none', fontSize: '0.8rem' }}>
                      <span>Run #{ex.attempt + 1} ({ex.worker_id?.slice(0, 8) || 'unknown'})</span>
                      <span>Started: {new Date(ex.started_at).toLocaleTimeString()}</span>
                      <span style={{ color: ex.outcome === 'success' ? '#34d399' : '#f87171', fontWeight: 600 }}>{ex.outcome?.toUpperCase() || 'RUNNING'}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div>
              <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', fontWeight: 500, marginBottom: '0.5rem' }}>Execution Logs</label>
              <div style={{ background: 'rgba(0,0,0,0.4)', padding: '0.75rem', borderRadius: '8px', maxHeight: '150px', overflowY: 'auto', fontFamily: 'monospace', fontSize: '0.75rem' }}>
                {selectedJob.logs.length === 0 ? (
                  <p style={{ color: 'var(--text-secondary)' }}>No stdout/stderr logs emitted.</p>
                ) : (
                  selectedJob.logs.map((l, i) => (
                    <div key={i} style={{ color: l.level === 'error' ? '#f87171' : l.level === 'warn' ? '#facc15' : '#9ca3af', marginBottom: '0.25rem' }}>
                      [{new Date(l.logged_at).toLocaleTimeString()}] {l.message}
                    </div>
                  ))
                )}
              </div>
            </div>

            <div style={{ display: 'flex', gap: '1rem', marginTop: '2rem', justifyContent: 'flex-end' }}>
              {(selectedJob.status === 'failed' || selectedJob.status === 'dead_letter') && (
                <button type="button" className="btn btn-primary" onClick={() => handleRetryJob(selectedJob.id)}>
                  <RefreshCw size={16} /> Re-enqueue Job
                </button>
              )}
              {(selectedJob.status === 'queued' || selectedJob.status === 'scheduled' || selectedJob.status === 'running') && (
                <button type="button" className="btn btn-danger" onClick={() => handleCancelJob(selectedJob.id)}>
                  Cancel Job
                </button>
              )}
              <button type="button" className="btn btn-secondary" onClick={() => setShowJobModal(false)}>Close</button>
            </div>
          </div>
        </div>
      )}

      {/* Global Command Palette (Cmd + K) */}
      <CommandPalette
        isOpen={showCmdPalette}
        onClose={() => setShowCmdPalette(false)}
        queues={queues}
        onSelectQueue={(q) => setSelectedQueue(q)}
        onNavigateTab={(tab) => setActiveTab(tab)}
        onOpenNewJob={() => setShowSubmitJobModal(true)}
        onOpenNewQueue={() => setShowNewQueueModal(true)}
        onReplayDLQ={handleBulkReplayDLQ}
      />
    </div>
  );
}
