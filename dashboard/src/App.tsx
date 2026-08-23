import React, { useState, useEffect } from 'react';
import { 
  Activity, Play, Pause, Layers, AlertCircle, RefreshCw, Cpu, 
  BarChart2, Plus, X, LogOut, CheckCircle, 
  FileText, Lock
} from 'lucide-react';
import { 
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, 
  ResponsiveContainer, BarChart, Bar
} from 'recharts';
import { api } from './api/client';
import type { Project, Queue, Job, JobDetails, WorkerInfo, SystemHealth, ThroughputPoint, DLQEntry } from './api/client';

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

  // WebSocket Live Updates Integration
  useEffect(() => {
    if (!isLoggedIn) return;
    const token = localStorage.getItem('token');
    if (!token) return;

    let ws: WebSocket | null = null;
    let reconnectTimeout: any = null;

    const connectWS = () => {
      try {
        const wsUrl = `ws://${window.location.hostname}:8080/api/ws?token=${encodeURIComponent(token)}`;
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
      setQueues(qList);
      if (qList.length > 0 && !selectedQueue) {
        setSelectedQueue(qList[0]);
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

  // Auth Screen
  if (!isLoggedIn) {
    return (
      <div className="auth-container">
        <div className="card auth-card">
          <div className="brand" style={{ justifyContent: 'center', marginBottom: '2rem' }}>
            <div className="brand-icon">O</div>
            <div className="brand-name">OBSIDIAN</div>
          </div>
          <h2 style={{ textAlign: 'center', marginBottom: '1.5rem', fontWeight: 500 }}>
            {isRegisterMode ? 'Create Account' : 'Welcome Back'}
          </h2>
          <form onSubmit={handleAuth}>
            {authError && <div style={{ color: '#ef4444', background: 'rgba(239, 68, 68, 0.1)', padding: '0.75rem', borderRadius: '8px', marginBottom: '1rem', fontSize: '0.875rem' }}>{authError}</div>}
            <div className="form-group">
              <label>Email Address</label>
              <input 
                type="email" 
                className="form-control" 
                value={email} 
                onChange={e => setEmail(e.target.value)} 
                required 
              />
            </div>
            <div className="form-group">
              <label>Password</label>
              <input 
                type="password" 
                className="form-control" 
                value={password} 
                onChange={e => setPassword(e.target.value)} 
                required 
              />
            </div>
            <button type="submit" className="btn btn-primary" style={{ width: '100%', marginTop: '1rem' }}>
              {isRegisterMode ? 'Sign Up' : 'Sign In'}
            </button>
          </form>
          <p style={{ textAlign: 'center', marginTop: '1.5rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
            {isRegisterMode ? 'Already have an account?' : "Don't have an account?"}{' '}
            <span 
              style={{ color: 'var(--accent-blue)', cursor: 'pointer', fontWeight: 500 }}
              onClick={() => setIsRegisterMode(!isRegisterMode)}
            >
              {isRegisterMode ? 'Sign In' : 'Sign Up'}
            </span>
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="app-container">
      {/* Sidebar Navigation */}
      <nav className="sidebar">
        <div className="brand">
          <div className="brand-icon">O</div>
          <div className="brand-name">OBSIDIAN</div>
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
          </li>
          <li className={`nav-item ${activeTab === 'metrics' ? 'active' : ''}`} onClick={() => setActiveTab('metrics')}>
            <BarChart2 size={18} /> Observability
          </li>
        </ul>

        <div style={{ marginTop: 'auto' }}>
          {selectedProject && (
            <div style={{ background: 'rgba(255,255,255,0.02)', padding: '0.75rem', borderRadius: '8px', marginBottom: '1rem', border: '1px dashed var(--border-color)' }}>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>API API Key:</p>
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
            <div style={{ background: 'rgba(255,255,255,0.06)', border: '1px solid var(--border-color)', padding: '0.35rem 0.75rem', borderRadius: '20px', fontSize: '0.8rem', fontWeight: 600, textTransform: 'capitalize', display: 'flex', alignItems: 'center', gap: '0.4rem', color: userRole === 'admin' ? '#facc15' : userRole === 'member' ? '#60a5fa' : '#9ca3af' }}>
              <span>{userRole === 'admin' ? '👑' : userRole === 'member' ? '🔧' : '👁'} Role: {userRole}</span>
            </div>

            <div className="live-indicator">
              <span className="live-dot" style={{ background: wsConnected ? '#10b981' : '#f59e0b', boxShadow: wsConnected ? '0 0 8px #10b981' : '0 0 8px #f59e0b' }}></span>
              <span style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', fontWeight: 500 }}>
                {wsConnected ? 'WebSocket Live' : 'Polling (HTTP)'}
              </span>
            </div>

            <button className="btn btn-secondary" onClick={refreshData}>
              <RefreshCw size={16} /> Refresh
            </button>
          </div>
        </header>

        {/* Tab content logic */}
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
                      <th>Priority</th>
                      <th>Concurrency Limit</th>
                      <th>Job Statistics</th>
                      <th>Status</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {queues.length === 0 ? (
                      <tr>
                        <td colSpan={6} style={{ textAlign: 'center', padding: '3rem 1rem' }}>
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
                            <span className={`badge ${q.is_paused ? 'badge-cancelled' : 'badge-completed'}`}>
                              {q.is_paused ? 'Paused' : 'Active'}
                            </span>
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
            <div className="card-title">Dead Letter Queue Entries ({dlqEntries.length})</div>
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
                  <select className="form-control" value={submitJobType} onChange={e => setSubmitJobType(e.target.value)}>
                    <option value="noop">noop (instant success)</option>
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
              <label>Arguments (Payload)</label>
              <pre style={{ background: 'rgba(0,0,0,0.3)', padding: '0.75rem', borderRadius: '8px', fontFamily: 'monospace', fontSize: '0.8rem', overflowX: 'auto' }}>
                {JSON.stringify(selectedJob.payload, null, 2)}
              </pre>
            </div>

            <div style={{ marginBottom: '1.5rem' }}>
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
              {(selectedJob.status === 'queued' || selectedJob.status === 'scheduled') && (
                <button type="button" className="btn btn-danger" onClick={() => handleCancelJob(selectedJob.id)}>
                  Cancel Job
                </button>
              )}
              <button type="button" className="btn btn-secondary" onClick={() => setShowJobModal(false)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
