import {
  Avatar,
  Badge,
  Breadcrumb,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Grid,
  Input,
  Layout,
  Menu,
  Message,
  Modal,
  Select,
  Space,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
} from '@arco-design/web-react';
import {
  IconApps,
  IconArchive,
  IconCopy,
  IconDashboard,
  IconDelete,
  IconEdit,
  IconEmail,
  IconFile,
  IconHome,
  IconLock,
  IconMenu,
  IconPlus,
  IconRefresh,
  IconRobot,
  IconSearch,
  IconSettings,
  IconThunderbolt,
  IconUser,
} from '@arco-design/web-react/icon';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

type PublicConfig = {
  domains: string[];
  accessMode: 'public' | 'private';
  requiresAccess: boolean;
  adminSetupRequired: boolean;
  retention: string;
};

type MailMessage = {
  id: string;
  from: string;
  to: string[];
  subject: string;
  text: string;
  html: string;
  headers: Record<string, string[]>;
  raw?: string;
  receivedAt: string;
};

type WebhookRule = {
  id?: string;
  name: string;
  enabled: boolean;
  url: string;
  domains?: string[];
  localParts?: string[];
  fromPattern?: string;
  subject?: string;
  codePattern: string;
  source: 'text' | 'html' | 'raw' | 'all';
};

type CodeProject = {
  id?: string;
  name: string;
  enabled: boolean;
  description?: string;
  domains?: string[];
  localParts?: string[];
  fromPattern?: string;
  subject?: string;
  codePattern: string;
  source: 'text' | 'html' | 'raw' | 'all';
};

type CodeMatch = {
  id: string;
  projectId: string;
  projectName: string;
  mailbox: string;
  code: string;
  subject: string;
  from: string;
  receivedAt: string;
  messageId: string;
};

type TestMessagePayload = Pick<MailMessage, 'from' | 'to' | 'subject' | 'text' | 'html'> & { raw: string };

type RegexSuggestion = {
  name: string;
  source: 'text' | 'html' | 'raw' | 'all';
  pattern: string;
  sampleCode: string;
  reason: string;
  confidence: number;
};

type CodeTestResponse = {
  matches: CodeMatch[];
  regexSuggestions?: RegexSuggestion[];
  aiError?: string;
};

type AdminConfig = {
  configPath: string;
  server: { httpAddr: string };
  smtp: { addr: string };
  mail: { domains: string[]; retention: string; reservedLocalParts: string[] };
  access: { mode: 'public' | 'private'; passwordSet: boolean };
  admin: { username: string; passwordSet: boolean };
  api: { enabled: boolean; tokenSet: boolean };
  openai: { enabled: boolean; apiKeySet: boolean; baseURL: string; model: string; timeout: string; apiMode: 'auto' | 'responses' | 'chat_completions' };
  webhooks: WebhookRule[];
  codeProjects: CodeProject[];
};

type SMTPDebugEvent = {
  id: string;
  time: string;
  type: string;
  sessionId?: string;
  remoteAddr?: string;
  localAddr?: string;
  helo?: string;
  from?: string;
  to?: string;
  recipients?: string[];
  messageId?: string;
  size?: number;
  error?: string;
  detail?: string;
};

type DNSIssue = {
  level: 'error' | 'warning' | 'info' | string;
  message: string;
};

type DNSAddress = {
  ip: string;
  version: 'A' | 'AAAA' | string;
  flags?: string[];
};

type DNSMX = {
  host: string;
  preference: number;
  isIP: boolean;
  addresses?: DNSAddress[];
  error?: string;
};

type DNSDomainReport = {
  domain: string;
  addresses?: DNSAddress[];
  mx?: DNSMX[];
  issues?: DNSIssue[];
};

type DNSDebugReport = {
  checkedAt: string;
  smtpAddr: string;
  smtpPort?: string;
  domains: DNSDomainReport[];
  issues?: DNSIssue[];
};

type ConfigDraft = {
  httpAddr: string;
  smtpAddr: string;
  domains: string;
  retention: string;
  reservedLocalParts: string;
  accessMode: 'public' | 'private';
  accessPassword: string;
  adminUsername: string;
  adminPassword: string;
  apiEnabled: boolean;
  apiToken: string;
  apiClearToken: boolean;
  openAIEnabled: boolean;
  openAIAPIKey: string;
  openAIClearAPIKey: boolean;
  openAIBaseURL: string;
  openAIModel: string;
  openAITimeout: string;
  openAIAPIMode: 'auto' | 'responses' | 'chat_completions';
};

type PageKey = 'mailbox' | 'detail' | 'adminLogin' | 'adminOverview' | 'settings' | 'codeProjects' | 'webhooks' | 'deliveryDebug' | 'allMessages';
type DetailMode = 'mailbox' | 'admin';

type AdminAuthForm = {
  username: string;
  password: string;
  accessPassword: string;
};

type ConfirmAction = {
  title: string;
  content: ReactNode;
  okText?: string;
  onOk: () => Promise<void>;
};

const { Sider, Header, Content } = Layout;
const { Row, Col } = Grid;
const Option = Select.Option;

const emptyPublicConfig: PublicConfig = {
  domains: ['localhost'],
  accessMode: 'public',
  requiresAccess: false,
  adminSetupRequired: true,
  retention: '1h',
};

const emptyRule: WebhookRule = {
  name: '',
  enabled: true,
  url: '',
  domains: [],
  localParts: [],
  fromPattern: '',
  subject: '',
  codePattern: '(\\d{6})',
  source: 'all',
};

const emptyCodeProject: CodeProject = {
  name: '',
  enabled: true,
  description: '',
  domains: [],
  localParts: [],
  fromPattern: '',
  subject: '',
  codePattern: '(\\d{6})',
  source: 'all',
};

const chatGPTCodeProject: CodeProject = {
  name: 'ChatGPT 注册',
  enabled: true,
  description: '自动识别 openai.com 邮件 HTML 中的 6 位验证码',
  domains: [],
  localParts: [],
  fromPattern: '(?i)openai\\.com',
  subject: '',
  codePattern: '(?is)<h1[^>]*>\\s*(\\d{6})\\s*</h1>|enter this code:\\s*(\\d{6})',
  source: 'html',
};

const chatGPTTemplateKey = '__chatgpt_template__';

const pageMeta: Record<PageKey, { title: string; group: string }> = {
  mailbox: { title: '收件台', group: 'Mailbox' },
  detail: { title: '邮件详情', group: 'Mailbox' },
  adminLogin: { title: '管理员登录', group: 'Admin' },
  adminOverview: { title: '管理概览', group: 'Admin' },
  settings: { title: '系统配置', group: 'Admin' },
  codeProjects: { title: '验证码项目', group: 'Admin' },
  webhooks: { title: 'WebHook 规则', group: 'Admin' },
  deliveryDebug: { title: '投递调试', group: 'Admin' },
  allMessages: { title: '全部邮件', group: 'Admin' },
};

async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new Error(payload.error || `HTTP ${response.status}`);
  }
  return payload as T;
}

function splitList(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function joinList(value?: string[]): string {
  return (value || []).join('\n');
}

function mailboxFromParts(localPart: string, domain: string): string {
  return `${localPart.trim().toLowerCase()}@${domain}`;
}

function formatDate(value?: string): string {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function containsMail(message: MailMessage, keyword: string): boolean {
  if (!keyword.trim()) return true;
  const text = `${message.subject} ${message.from} ${message.to.join(' ')} ${message.text}`.toLowerCase();
  return text.includes(keyword.trim().toLowerCase());
}

function filterSMTPEvents(events: SMTPDebugEvent[], keyword: string, type: string): SMTPDebugEvent[] {
  const normalizedKeyword = keyword.trim().toLowerCase();
  return events.filter((event) => {
    if (type && event.type !== type) return false;
    if (!normalizedKeyword) return true;
    const text = [
      event.remoteAddr,
      event.localAddr,
      event.helo,
      event.from,
      event.to,
      event.recipients?.join(' '),
      event.messageId,
      event.error,
      event.detail,
    ].join(' ').toLowerCase();
    return text.includes(normalizedKeyword);
  });
}

function smtpEventLabel(type: string): string {
  const labels: Record<string, string> = {
    listen_start: '监听启动',
    listen_error: '监听失败',
    connect: 'TCP 连接',
    helo: 'HELO/EHLO',
    mail_from: 'MAIL FROM',
    rcpt_accept: 'RCPT 接受',
    rcpt_reject: 'RCPT 拒收',
    data_start: 'DATA 开始',
    data_error: 'DATA 失败',
    mail_stored: '邮件写入',
  };
  return labels[type] || type || '未知';
}

function smtpEventColor(type: string): string {
  if (type.includes('error') || type.includes('reject')) return 'red';
  if (type === 'mail_stored') return 'green';
  if (type === 'data_start' || type === 'mail_from' || type === 'rcpt_accept') return 'arcoblue';
  if (type === 'listen_start' || type === 'connect' || type === 'helo') return 'cyan';
  return 'gray';
}

function formatBytes(size?: number): string {
  if (!size) return '-';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function configDraftFrom(cfg: AdminConfig): ConfigDraft {
  return {
    httpAddr: cfg.server.httpAddr,
    smtpAddr: cfg.smtp.addr,
    domains: joinList(cfg.mail.domains),
    retention: cfg.mail.retention,
    reservedLocalParts: joinList(cfg.mail.reservedLocalParts),
    accessMode: cfg.access.mode,
    accessPassword: '',
    adminUsername: cfg.admin.username,
    adminPassword: '',
    apiEnabled: cfg.api.enabled,
    apiToken: '',
    apiClearToken: false,
    openAIEnabled: cfg.openai.enabled,
    openAIAPIKey: '',
    openAIClearAPIKey: false,
    openAIBaseURL: cfg.openai.baseURL,
    openAIModel: cfg.openai.model,
    openAITimeout: cfg.openai.timeout,
    openAIAPIMode: cfg.openai.apiMode || 'auto',
  };
}

function isAdminPage(page: PageKey): boolean {
  return page === 'adminOverview' || page === 'settings' || page === 'codeProjects' || page === 'webhooks' || page === 'deliveryDebug' || page === 'allMessages';
}

function App() {
  const [activePage, setActivePage] = useState<PageKey>('mailbox');
  const [mobileMenuVisible, setMobileMenuVisible] = useState(false);
  const [publicConfig, setPublicConfig] = useState<PublicConfig>(emptyPublicConfig);
  const [loadingConfig, setLoadingConfig] = useState(true);
  const [accessGranted, setAccessGranted] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');

  const [domain, setDomain] = useState(emptyPublicConfig.domains[0]);
  const [localPart, setLocalPart] = useState(localStorage.getItem('lemail.localPart') || '');
  const [mailboxMessages, setMailboxMessages] = useState<MailMessage[]>([]);
  const [codeMatches, setCodeMatches] = useState<CodeMatch[]>([]);
  const [selectedMessage, setSelectedMessage] = useState<MailMessage | null>(null);
  const [detailMode, setDetailMode] = useState<DetailMode>('mailbox');
  const [mailLoading, setMailLoading] = useState(false);
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false);

  const [adminReady, setAdminReady] = useState(false);
  const [adminConfig, setAdminConfig] = useState<AdminConfig | null>(null);
  const [adminMessages, setAdminMessages] = useState<MailMessage[]>([]);
  const [adminCodes, setAdminCodes] = useState<CodeMatch[]>([]);
  const [adminLoading, setAdminLoading] = useState(false);
  const [smtpEvents, setSMTPEvents] = useState<SMTPDebugEvent[]>([]);
  const [dnsReport, setDNSReport] = useState<DNSDebugReport | null>(null);
  const [debugLoading, setDebugLoading] = useState(false);
  const [debugMailboxFilter, setDebugMailboxFilter] = useState('');
  const [debugTypeFilter, setDebugTypeFilter] = useState('');
  const [configDraft, setConfigDraft] = useState<ConfigDraft | null>(null);
  const [authForm, setAuthForm] = useState<AdminAuthForm>({ username: 'admin', password: '', accessPassword: '' });
  const [ruleDraft, setRuleDraft] = useState<WebhookRule>(emptyRule);
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null);
  const [ruleModalVisible, setRuleModalVisible] = useState(false);
  const [projectDraft, setProjectDraft] = useState<CodeProject>(emptyCodeProject);
  const [editingProjectId, setEditingProjectId] = useState<string | null>(null);
  const [projectModalVisible, setProjectModalVisible] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const currentAddress = useMemo(() => (localPart ? mailboxFromParts(localPart, domain) : ''), [localPart, domain]);
  const currentAddressRef = useRef(currentAddress);
  const filteredMailboxMessages = useMemo(
    () => mailboxMessages.filter((message) => containsMail(message, searchKeyword)),
    [mailboxMessages, searchKeyword],
  );
  const filteredAdminMessages = useMemo(
    () => adminMessages.filter((message) => containsMail(message, searchKeyword)),
    [adminMessages, searchKeyword],
  );
  const filteredSMTPEvents = useMemo(
    () => filterSMTPEvents(smtpEvents, debugMailboxFilter, debugTypeFilter),
    [smtpEvents, debugMailboxFilter, debugTypeFilter],
  );

  const refreshPublicConfig = async () => {
    setLoadingConfig(true);
    try {
      const cfg = await apiRequest<PublicConfig>('/api/public/config');
      setPublicConfig(cfg);
      setDomain((current) => (cfg.domains.includes(current) ? current : cfg.domains[0] || 'localhost'));
      setAccessGranted((current) => (cfg.requiresAccess ? current : true));
    } catch (error) {
      Message.error((error as Error).message);
    } finally {
      setLoadingConfig(false);
    }
  };

  const loadAdminData = async (silent = false) => {
    if (!silent) setAdminLoading(true);
    try {
      const [cfg, allMessages, allCodes] = await Promise.all([
        apiRequest<AdminConfig>('/api/admin/config'),
        apiRequest<{ messages: MailMessage[] }>('/api/admin/messages'),
        apiRequest<{ codes: CodeMatch[] }>('/api/admin/codes'),
      ]);
      setAdminConfig(cfg);
      setConfigDraft(configDraftFrom(cfg));
      setAdminMessages(allMessages.messages || []);
      setAdminCodes(allCodes.codes || []);
      setAdminReady(true);
    } catch (error) {
      if (!silent) Message.error((error as Error).message);
      setAdminReady(false);
    } finally {
      if (!silent) setAdminLoading(false);
    }
  };

  const loadDeliveryDebug = async () => {
    if (!adminReady) return;
    setDebugLoading(true);
    try {
      const [eventsPayload, dnsPayload] = await Promise.all([
        apiRequest<{ events: SMTPDebugEvent[] }>('/api/admin/debug/smtp/events'),
        apiRequest<DNSDebugReport>('/api/admin/debug/dns'),
      ]);
      setSMTPEvents(eventsPayload.events || []);
      setDNSReport(dnsPayload);
    } catch (error) {
      Message.error((error as Error).message);
    } finally {
      setDebugLoading(false);
    }
  };

  const clearDeliveryDebug = () => {
    setConfirmAction({
      title: '清空投递调试事件？',
      content: '只会清空内存中的 SMTP 调试事件，不会删除邮件或验证码。',
      okText: '确认清空',
      onOk: async () => {
        await apiRequest('/api/admin/debug/smtp/events', { method: 'DELETE' });
        setSMTPEvents([]);
        Message.success('投递调试事件已清空');
      },
    });
  };

  useEffect(() => {
    refreshPublicConfig();
    loadAdminData(true);
  }, []);

  useEffect(() => {
    if (localPart) localStorage.setItem('lemail.localPart', localPart);
  }, [localPart]);

  useEffect(() => {
    currentAddressRef.current = currentAddress;
  }, [currentAddress]);

  useEffect(() => {
    if (adminReady && activePage === 'adminLogin') {
      setActivePage('adminOverview');
    }
  }, [activePage, adminReady]);

  useEffect(() => {
    if (adminReady && activePage === 'deliveryDebug') {
      loadDeliveryDebug();
    }
  }, [activePage, adminReady]);

  const loadMailboxMessages = async (mailbox = currentAddress) => {
    if (!mailbox) return;
    setMailLoading(true);
    try {
      const data = await apiRequest<{ messages: MailMessage[] }>(`/api/mailbox/${encodeURIComponent(mailbox)}/messages`);
      if (mailbox !== currentAddressRef.current) return;
      setMailboxMessages(data.messages || []);
      setSelectedMessage((current) => current || data.messages?.[0] || null);
    } catch (error) {
      Message.error((error as Error).message);
    } finally {
      setMailLoading(false);
    }
  };

  const loadMailboxCodes = async (mailbox = currentAddress) => {
    if (!mailbox) {
      setCodeMatches([]);
      return;
    }
    try {
      const data = await apiRequest<{ codes: CodeMatch[] }>(`/api/mailbox/${encodeURIComponent(mailbox)}/codes`);
      if (mailbox !== currentAddressRef.current) return;
      setCodeMatches(data.codes || []);
    } catch (error) {
      Message.error((error as Error).message);
    }
  };

  useEffect(() => {
    if (!currentAddress || !accessGranted) {
      if (!currentAddress) {
        setMailboxMessages([]);
        setCodeMatches([]);
      }
      return;
    }
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/mailbox?address=${encodeURIComponent(currentAddress)}`);
    const refreshMailbox = () => {
      loadMailboxMessages(currentAddress);
      loadMailboxCodes(currentAddress);
    };
    refreshMailbox();
    socket.onopen = refreshMailbox;
    socket.onerror = refreshMailbox;
    socket.onmessage = (event) => {
      const payload = JSON.parse(event.data) as { type: string; message: MailMessage; codes?: CodeMatch[] };
      if (payload.type === 'mail') {
        setMailboxMessages((items) => [payload.message, ...items.filter((item) => item.id !== payload.message.id)]);
        if (payload.codes) {
          setCodeMatches(payload.codes);
          const extractedCodes = payload.codes.filter((item) => item.messageId === payload.message.id);
          if (extractedCodes.length > 0) {
            Message.success(`收到新邮件，已自动提取 ${extractedCodes.length} 个验证码`);
          } else {
            Message.info(`收到来自 ${payload.message.from || 'unknown'} 的新邮件，已更新收件箱`);
          }
        } else {
          loadMailboxCodes(currentAddress);
          Message.info(`收到来自 ${payload.message.from || 'unknown'} 的新邮件，已更新收件箱`);
        }
      }
    };
    return () => socket.close();
  }, [currentAddress, accessGranted]);

  const unlockAccess = async (password: string) => {
    await apiRequest('/api/access/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    });
    setAccessGranted(true);
    Message.success('访问已解锁');
  };

  const randomMailbox = async () => {
    setMailLoading(true);
    try {
      const data = await apiRequest<{ localPart: string; domain: string; address: string }>('/api/mailbox/random', {
        method: 'POST',
        body: JSON.stringify({ domain }),
      });
      setLocalPart(data.localPart);
      setDomain(data.domain);
      await navigator.clipboard?.writeText(data.address);
      Message.success('已生成并复制随机邮箱');
    } catch (error) {
      Message.error((error as Error).message);
    } finally {
      setMailLoading(false);
    }
  };

  const copyAddress = async () => {
    if (!currentAddress) return;
    await navigator.clipboard?.writeText(currentAddress);
    Message.success('邮箱地址已复制');
  };

  const copyCode = async (code: string) => {
    await navigator.clipboard?.writeText(code);
    Message.success(`验证码 ${code} 已复制`);
  };

  const testCodeProject = async (messageId: string, project: CodeProject, message: TestMessagePayload, suggestRegex = false) => {
    return apiRequest<CodeTestResponse>('/api/admin/code-projects/test', {
      method: 'POST',
      body: JSON.stringify({ messageId, project, message, suggestRegex }),
    });
  };

  const openMessage = (message: MailMessage, preferPage = false, mode?: DetailMode) => {
    setSelectedMessage(message);
    setDetailMode(mode || (isAdminPage(activePage) ? 'admin' : 'mailbox'));
    if (preferPage) {
      setActivePage('detail');
      return;
    }
    setDetailDrawerVisible(true);
  };

  const adminAuth = async () => {
    try {
      const endpoint = publicConfig.adminSetupRequired ? '/api/admin/setup' : '/api/admin/login';
      await apiRequest(endpoint, { method: 'POST', body: JSON.stringify(authForm) });
      Message.success(publicConfig.adminSetupRequired ? '管理员初始化完成' : '管理员已登录');
      await refreshPublicConfig();
      await loadAdminData();
      setActivePage('adminOverview');
    } catch (error) {
      Message.error((error as Error).message);
    }
  };

  const saveConfig = async () => {
    if (!configDraft || !adminConfig) return;
    try {
      const payload = {
        server: { httpAddr: configDraft.httpAddr },
        smtp: { addr: configDraft.smtpAddr },
        mail: {
          domains: splitList(configDraft.domains),
          retention: configDraft.retention,
          reservedLocalParts: splitList(configDraft.reservedLocalParts),
        },
        access: { mode: configDraft.accessMode, password: configDraft.accessPassword },
        admin: { username: configDraft.adminUsername, password: configDraft.adminPassword },
        api: {
          enabled: configDraft.apiEnabled,
          token: configDraft.apiToken,
          clearToken: configDraft.apiClearToken,
        },
        openai: {
          enabled: configDraft.openAIEnabled,
          apiKey: configDraft.openAIAPIKey,
          clearApiKey: configDraft.openAIClearAPIKey,
          baseURL: configDraft.openAIBaseURL,
          model: configDraft.openAIModel,
          timeout: configDraft.openAITimeout,
          apiMode: configDraft.openAIAPIMode,
        },
        webhooks: adminConfig.webhooks || [],
        codeProjects: adminConfig.codeProjects || [],
      };
      const cfg = await apiRequest<AdminConfig>('/api/admin/config', { method: 'PUT', body: JSON.stringify(payload) });
      setAdminConfig(cfg);
      setConfigDraft(configDraftFrom(cfg));
      await refreshPublicConfig();
      Message.success('配置已保存');
    } catch (error) {
      Message.error((error as Error).message);
    }
  };

  const openRuleModal = (rule?: WebhookRule) => {
    setRuleDraft(rule ? { ...emptyRule, ...rule } : emptyRule);
    setEditingRuleId(rule?.id || null);
    setRuleModalVisible(true);
  };

  const saveRule = async () => {
    try {
      const payload = { ...ruleDraft, domains: ruleDraft.domains || [], localParts: ruleDraft.localParts || [] };
      if (editingRuleId) {
        await apiRequest(`/api/admin/webhooks/${editingRuleId}`, { method: 'PUT', body: JSON.stringify(payload) });
      } else {
        await apiRequest('/api/admin/webhooks', { method: 'POST', body: JSON.stringify(payload) });
      }
      setRuleModalVisible(false);
      setEditingRuleId(null);
      setRuleDraft(emptyRule);
      await loadAdminData();
      Message.success('WebHook 规则已保存');
    } catch (error) {
      Message.error((error as Error).message);
    }
  };

  const runConfirmAction = async () => {
    if (!confirmAction) return;
    setConfirmLoading(true);
    try {
      await confirmAction.onOk();
      setConfirmAction(null);
    } catch (error) {
      Message.error((error as Error).message);
    } finally {
      setConfirmLoading(false);
    }
  };

  const deleteRule = (id?: string) => {
    if (!id) return;
    setConfirmAction({
      title: '删除 WebHook 规则？',
      content: '删除后验证码将不再发送到该 WebHook。',
      okText: '确认删除',
      onOk: async () => {
        await apiRequest(`/api/admin/webhooks/${id}`, { method: 'DELETE' });
        await loadAdminData();
        Message.success('规则已删除');
      },
    });
  };

  const openProjectModal = (project?: CodeProject) => {
    setProjectDraft(project ? { ...emptyCodeProject, ...project } : emptyCodeProject);
    setEditingProjectId(project?.id || null);
    setProjectModalVisible(true);
  };

  const openChatGPTProjectModal = () => {
    setProjectDraft(chatGPTCodeProject);
    setEditingProjectId(null);
    setProjectModalVisible(true);
  };

  const saveProject = async () => {
    try {
      const payload = { ...projectDraft, domains: projectDraft.domains || [], localParts: projectDraft.localParts || [] };
      if (editingProjectId) {
        await apiRequest(`/api/admin/code-projects/${editingProjectId}`, { method: 'PUT', body: JSON.stringify(payload) });
      } else {
        await apiRequest('/api/admin/code-projects', { method: 'POST', body: JSON.stringify(payload) });
      }
      setProjectModalVisible(false);
      setEditingProjectId(null);
      setProjectDraft(emptyCodeProject);
      await loadAdminData();
      if (currentAddress) await loadMailboxCodes(currentAddress);
      Message.success('验证码项目已保存');
    } catch (error) {
      Message.error((error as Error).message);
    }
  };

  const deleteProject = (id?: string) => {
    if (!id) return;
    setConfirmAction({
      title: '删除验证码项目？',
      content: '删除后将不再自动提取该项目的验证码，已有内存结果会重新计算。',
      okText: '确认删除',
      onOk: async () => {
        await apiRequest(`/api/admin/code-projects/${id}`, { method: 'DELETE' });
        await loadAdminData();
        if (currentAddress) await loadMailboxCodes(currentAddress);
        Message.success('验证码项目已删除');
      },
    });
  };

  if (publicConfig.requiresAccess && !accessGranted) {
    return <AccessGate onSubmit={unlockAccess} />;
  }

  if (!adminReady && (activePage === 'adminLogin' || isAdminPage(activePage))) {
    return (
      <AdminLoginScreen
        publicConfig={publicConfig}
        authForm={authForm}
        onAuthFormChange={setAuthForm}
        onSubmit={adminAuth}
        onBack={() => setActivePage('mailbox')}
      />
    );
  }

  const menu = (
    <SideMenu
      activePage={activePage}
      adminReady={adminReady}
      onChange={(key) => {
        setActivePage(key);
        setMobileMenuVisible(false);
      }}
    />
  );

  return (
    <Layout className="console-shell">
      <Sider className="console-sider" width={220} collapsed={false}>
        {menu}
      </Sider>
      <Drawer
        className="mobile-menu-drawer"
        width={260}
        title={null}
        footer={null}
        visible={mobileMenuVisible}
        onCancel={() => setMobileMenuVisible(false)}
      >
        {menu}
      </Drawer>
      <Layout>
        <Header className="console-header">
          <Button className="mobile-menu-button" type="text" icon={<IconMenu />} onClick={() => setMobileMenuVisible(true)} />
          <Input
            className="top-search"
            allowClear
            prefix={<IconSearch />}
            placeholder="搜索邮件主题、发件人或收件人"
            value={searchKeyword}
            onChange={setSearchKeyword}
          />
          <Space className="header-actions" size="medium">
            <Tag color="arcoblue">{currentAddress || 'No mailbox'}</Tag>
            <Badge status={publicConfig.requiresAccess ? 'warning' : 'success'} text={publicConfig.requiresAccess ? 'Private' : 'Public'} />
            <Tag color="gray">TTL {publicConfig.retention}</Tag>
            <Button icon={<IconRefresh />} loading={loadingConfig || mailLoading || adminLoading} onClick={() => {
              refreshPublicConfig();
              if (currentAddress) {
                loadMailboxMessages();
                loadMailboxCodes();
              }
              if (adminReady) loadAdminData(true);
              if (adminReady && activePage === 'deliveryDebug') loadDeliveryDebug();
            }} />
            <Avatar className="admin-avatar" onClick={() => setActivePage(adminReady ? 'adminOverview' : 'adminLogin')}>
              <IconUser />
            </Avatar>
          </Space>
        </Header>
        <Content className="console-content">
          <Breadcrumb className="console-breadcrumb">
            <Breadcrumb.Item><IconHome /> LeMail</Breadcrumb.Item>
            <Breadcrumb.Item>{pageMeta[activePage].group}</Breadcrumb.Item>
            <Breadcrumb.Item>{pageMeta[activePage].title}</Breadcrumb.Item>
          </Breadcrumb>
          <PageHeader title={pageMeta[activePage].title} description="基于 ArcoDesign 的临时邮箱管理中台" />
          {activePage === 'mailbox' && (
            <MailboxPage
              publicConfig={publicConfig}
              domain={domain}
              localPart={localPart}
              currentAddress={currentAddress}
              messages={filteredMailboxMessages}
              codes={codeMatches}
              loading={mailLoading}
              onDomainChange={setDomain}
              onLocalPartChange={setLocalPart}
              onRandom={randomMailbox}
              onCopy={copyAddress}
              onCopyCode={copyCode}
              onRefresh={() => {
                loadMailboxMessages();
                loadMailboxCodes();
              }}
              onOpenMessage={openMessage}
            />
          )}
          {activePage === 'detail' && (
            <MailDetailPage
              message={selectedMessage}
              showCodeTester={adminReady && detailMode === 'admin'}
              codeProjects={adminConfig?.codeProjects || []}
              openAI={adminConfig?.openai}
              onTestCodeProject={testCodeProject}
              onCopyCode={copyCode}
            />
          )}
          {activePage === 'adminOverview' && (
            <AdminGuard adminReady={adminReady} publicConfig={publicConfig} authForm={authForm} onAuthFormChange={setAuthForm} onSubmit={adminAuth}>
              <AdminOverview publicConfig={publicConfig} adminConfig={adminConfig} adminMessages={adminMessages} adminCodes={adminCodes} onRefresh={() => loadAdminData()} />
            </AdminGuard>
          )}
          {activePage === 'settings' && (
            <AdminGuard adminReady={adminReady} publicConfig={publicConfig} authForm={authForm} onAuthFormChange={setAuthForm} onSubmit={adminAuth}>
              <SettingsPage adminConfig={adminConfig} draft={configDraft} setDraft={setConfigDraft} onSave={saveConfig} />
            </AdminGuard>
          )}
          {activePage === 'codeProjects' && (
            <AdminGuard adminReady={adminReady} publicConfig={publicConfig} authForm={authForm} onAuthFormChange={setAuthForm} onSubmit={adminAuth}>
              <CodeProjectsPage
                adminConfig={adminConfig}
                codes={adminCodes}
                onCreate={() => openProjectModal()}
                onCreateChatGPT={openChatGPTProjectModal}
                onEdit={openProjectModal}
                onDelete={deleteProject}
                onCopyCode={copyCode}
              />
            </AdminGuard>
          )}
          {activePage === 'webhooks' && (
            <AdminGuard adminReady={adminReady} publicConfig={publicConfig} authForm={authForm} onAuthFormChange={setAuthForm} onSubmit={adminAuth}>
              <WebhooksPage adminConfig={adminConfig} onCreate={() => openRuleModal()} onEdit={openRuleModal} onDelete={deleteRule} />
            </AdminGuard>
          )}
          {activePage === 'deliveryDebug' && (
            <AdminGuard adminReady={adminReady} publicConfig={publicConfig} authForm={authForm} onAuthFormChange={setAuthForm} onSubmit={adminAuth}>
              <DeliveryDebugPage
                events={filteredSMTPEvents}
                allEvents={smtpEvents}
                dnsReport={dnsReport}
                loading={debugLoading}
                mailboxFilter={debugMailboxFilter}
                typeFilter={debugTypeFilter}
                onMailboxFilterChange={setDebugMailboxFilter}
                onTypeFilterChange={setDebugTypeFilter}
                onRefresh={loadDeliveryDebug}
                onClear={clearDeliveryDebug}
              />
            </AdminGuard>
          )}
          {activePage === 'allMessages' && (
            <AdminGuard adminReady={adminReady} publicConfig={publicConfig} authForm={authForm} onAuthFormChange={setAuthForm} onSubmit={adminAuth}>
              <AllMessagesPage messages={filteredAdminMessages} loading={adminLoading} onOpenMessage={openMessage} onRefresh={() => loadAdminData()} />
            </AdminGuard>
          )}
        </Content>
      </Layout>
      <MessageDrawer
        message={selectedMessage}
        visible={detailDrawerVisible}
        showCodeTester={adminReady && detailMode === 'admin'}
        codeProjects={adminConfig?.codeProjects || []}
        openAI={adminConfig?.openai}
        onTestCodeProject={testCodeProject}
        onCopyCode={copyCode}
        onClose={() => setDetailDrawerVisible(false)}
        onOpenPage={() => {
          setDetailDrawerVisible(false);
          setActivePage('detail');
        }}
      />
      <WebhookModal
        visible={ruleModalVisible}
        rule={ruleDraft}
        editing={Boolean(editingRuleId)}
        onChange={setRuleDraft}
        onCancel={() => setRuleModalVisible(false)}
        onSave={saveRule}
      />
      <CodeProjectModal
        visible={projectModalVisible}
        project={projectDraft}
        editing={Boolean(editingProjectId)}
        onChange={setProjectDraft}
        onCancel={() => setProjectModalVisible(false)}
        onSave={saveProject}
      />
      <Modal
        title={confirmAction?.title || ''}
        visible={Boolean(confirmAction)}
        confirmLoading={confirmLoading}
        okText={confirmAction?.okText || '确认'}
        cancelText="取消"
        okButtonProps={{ status: 'danger' }}
        onCancel={() => {
          if (!confirmLoading) setConfirmAction(null);
        }}
        onOk={runConfirmAction}
      >
        <Typography.Paragraph className="confirm-copy">
          {confirmAction?.content}
        </Typography.Paragraph>
      </Modal>
    </Layout>
  );
}

function SideMenu({ activePage, adminReady, onChange }: { activePage: PageKey; adminReady: boolean; onChange: (key: PageKey) => void }) {
  return (
    <div className="side-wrap">
      <div className="brand">
        <div className="brand-logo"><IconRobot /></div>
        <div>
          <div className="brand-title">LeMail</div>
          <div className="brand-subtitle">Mail Console</div>
        </div>
      </div>
      <Menu className="side-menu" selectedKeys={[activePage]} onClickMenuItem={(key) => onChange(key as PageKey)}>
        <Menu.Item key="mailbox"><IconEmail />收件台</Menu.Item>
        <Menu.Item key="detail"><IconFile />邮件详情</Menu.Item>
        {adminReady ? (
          <>
            <Menu.Item key="adminOverview"><IconDashboard />管理概览</Menu.Item>
            <Menu.Item key="settings"><IconSettings />系统配置</Menu.Item>
            <Menu.Item key="codeProjects"><IconRobot />验证码项目</Menu.Item>
            <Menu.Item key="webhooks"><IconThunderbolt />WebHook 规则</Menu.Item>
            <Menu.Item key="deliveryDebug"><IconSearch />投递调试</Menu.Item>
            <Menu.Item key="allMessages"><IconArchive />全部邮件</Menu.Item>
          </>
        ) : (
          <Menu.Item key="adminLogin"><IconUser />管理员登录</Menu.Item>
        )}
      </Menu>
      <div className="side-status">
        <Badge status={adminReady ? 'success' : 'default'} text={adminReady ? 'Admin online' : 'Admin locked'} />
      </div>
    </div>
  );
}

function PageHeader({ title, description }: { title: string; description: string }) {
  return (
    <div className="page-heading">
      <div>
        <Typography.Title heading={3}>{title}</Typography.Title>
        <Typography.Text type="secondary">{description}</Typography.Text>
      </div>
    </div>
  );
}

function AdminLoginScreen({ publicConfig, authForm, onAuthFormChange, onSubmit, onBack }: {
  publicConfig: PublicConfig;
  authForm: AdminAuthForm;
  onAuthFormChange: (value: AdminAuthForm) => void;
  onSubmit: () => void;
  onBack: () => void;
}) {
  return (
    <div className="admin-login-screen">
      <div className="admin-login-hero">
        <div className="brand standalone">
          <div className="brand-logo"><IconRobot /></div>
          <div>
            <div className="brand-title">LeMail</div>
            <div className="brand-subtitle">Admin Console</div>
          </div>
        </div>
        <Typography.Title heading={2}>{publicConfig.adminSetupRequired ? '初始化管理员' : '管理员登录'}</Typography.Title>
        <Typography.Text type="secondary">
          管理后台独立登录，登录成功后才显示管理概览、系统配置、验证码项目、WebHook 规则、投递调试和全部邮件菜单。
        </Typography.Text>
      </div>
      <AdminAuthCard
        publicConfig={publicConfig}
        authForm={authForm}
        onAuthFormChange={onAuthFormChange}
        onSubmit={onSubmit}
        onBack={onBack}
      />
    </div>
  );
}

function AccessGate({ onSubmit }: { onSubmit: (password: string) => Promise<void> }) {
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const submit = async () => {
    setLoading(true);
    try {
      await onSubmit(password);
    } finally {
      setLoading(false);
    }
  };
  return (
    <div className="access-screen">
      <Card className="access-card" bordered={false}>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <div className="access-logo"><IconLock /></div>
          <div>
            <Typography.Title heading={3}>私有访问</Typography.Title>
            <Typography.Text type="secondary">当前实例需要访问密码，解锁后才能进入临时邮箱中台。</Typography.Text>
          </div>
          <Input.Password placeholder="访问密码" value={password} onChange={setPassword} onPressEnter={submit} />
          <Button type="primary" long loading={loading} onClick={submit}>进入控制台</Button>
        </Space>
      </Card>
    </div>
  );
}

function MailboxPage(props: {
  publicConfig: PublicConfig;
  domain: string;
  localPart: string;
  currentAddress: string;
  messages: MailMessage[];
  codes: CodeMatch[];
  loading: boolean;
  onDomainChange: (value: string) => void;
  onLocalPartChange: (value: string) => void;
  onRandom: () => void;
  onCopy: () => void;
  onCopyCode: (code: string) => void;
  onRefresh: () => void;
  onOpenMessage: (message: MailMessage, preferPage?: boolean) => void;
}) {
  const columns = [
    { title: '主题', dataIndex: 'subject', render: (value: string) => value || '无主题' },
    { title: '发件人', dataIndex: 'from', ellipsis: true },
    { title: '收件人', render: (_: unknown, item: MailMessage) => item.to.join(', ') },
    { title: '时间', render: (_: unknown, item: MailMessage) => formatDate(item.receivedAt) },
    {
      title: '操作',
      width: 120,
      render: (_: unknown, item: MailMessage) => <Button size="small" type="text" onClick={() => props.onOpenMessage(item)}>查看</Button>,
    },
  ];

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}><MetricCard title="当前邮箱" value={props.currentAddress || '未设置'} icon={<IconEmail />} /></Col>
        <Col xs={24} sm={12} lg={6}><MetricCard title="邮件数量" value={props.messages.length} icon={<IconArchive />} /></Col>
        <Col xs={24} sm={12} lg={6}><MetricCard title="已提取验证码" value={props.codes.length} icon={<IconRobot />} /></Col>
        <Col xs={24} sm={12} lg={6}><MetricCard title="访问模式" value={props.publicConfig.requiresAccess ? 'Private' : 'Public'} icon={<IconLock />} /></Col>
      </Row>
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={8}>
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            <Card className="console-card" title="邮箱配置" bordered={false}>
              <Form layout="vertical">
                <Form.Item label="邮箱前缀">
                  <Input
                    placeholder="local-part"
                    value={props.localPart}
                    onChange={(value) => props.onLocalPartChange(value.trim().toLowerCase())}
                  />
                </Form.Item>
                <Form.Item label="收信域名">
                  <Select value={props.domain} onChange={props.onDomainChange}>
                    {props.publicConfig.domains.map((item) => <Option key={item} value={item}>@{item}</Option>)}
                  </Select>
                </Form.Item>
                <div className="mail-address-card">
                  <div className="label">Mailbox</div>
                  <div className="value">{props.currentAddress || '生成或输入邮箱前缀'}</div>
                </div>
                <Space wrap>
                  <Button type="primary" icon={<IconRefresh />} loading={props.loading} onClick={props.onRandom}>随机邮箱</Button>
                  <Button icon={<IconCopy />} disabled={!props.currentAddress} onClick={props.onCopy}>复制地址</Button>
                  <Button disabled={!props.currentAddress} onClick={props.onRefresh}>刷新</Button>
                </Space>
              </Form>
            </Card>
            <CodeMatchPanel codes={props.codes} onCopyCode={props.onCopyCode} />
          </Space>
        </Col>
        <Col xs={24} lg={16}>
          <Card className="console-card" title="收件箱" bordered={false} extra={<Tag color="arcoblue">实时推送</Tag>}>
            <Table
              rowKey="id"
              loading={props.loading}
              data={props.messages}
              columns={columns}
              pagination={{ pageSize: 8 }}
              noDataElement={<Empty description="还没有邮件，向当前地址发送一封试试。" />}
              onRow={(record) => ({ onClick: () => props.onOpenMessage(record as MailMessage) })}
            />
          </Card>
        </Col>
      </Row>
    </Space>
  );
}

function MetricCard({ title, value, icon }: { title: string | number; value: string | number; icon: ReactNode }) {
  return (
    <Card className="metric-card" bordered={false}>
      <div className="metric-icon">{icon}</div>
      <Statistic title={title} value={value} styleValue={{ fontSize: 24, fontWeight: 700 }} />
    </Card>
  );
}

function CodeMatchPanel({ codes, onCopyCode }: { codes: CodeMatch[]; onCopyCode: (code: string) => void }) {
  return (
    <Card
      className="console-card"
      title="自动提取验证码"
      bordered={false}
      extra={<Space size="small"><Tag color="green">Auto</Tag><Tag>{codes.length} 条</Tag></Space>}
    >
      {codes.length === 0 ? (
        <Empty description="暂无验证码，管理员可创建 ChatGPT 等项目后自动识别。" />
      ) : (
        <div className="code-scroll-list">
          {codes.map((item) => (
            <div className="code-compact-card" key={item.id}>
              <div className="code-compact-head">
                <Tag color="arcoblue">{item.projectName}</Tag>
                <Typography.Text type="secondary">{formatDate(item.receivedAt)}</Typography.Text>
              </div>
              <div className="code-compact-body">
                <span className="code-inline code-compact-value">{item.code}</span>
                <Button size="small" type="primary" icon={<IconCopy />} onClick={() => onCopyCode(item.code)}>复制</Button>
              </div>
              <Typography.Text type="secondary" className="code-meta">{item.from || 'unknown'}</Typography.Text>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function MailDetailPage(props: {
  message: MailMessage | null;
  showCodeTester: boolean;
  codeProjects: CodeProject[];
  openAI?: AdminConfig['openai'];
  onTestCodeProject: (messageId: string, project: CodeProject, message: TestMessagePayload, useAI?: boolean) => Promise<CodeTestResponse>;
  onCopyCode: (code: string) => void;
}) {
  return (
    <Card className="console-card" bordered={false}>
      <MailContent {...props} />
    </Card>
  );
}

function MessageDrawer(props: {
  message: MailMessage | null;
  visible: boolean;
  showCodeTester: boolean;
  codeProjects: CodeProject[];
  openAI?: AdminConfig['openai'];
  onTestCodeProject: (messageId: string, project: CodeProject, message: TestMessagePayload, useAI?: boolean) => Promise<CodeTestResponse>;
  onCopyCode: (code: string) => void;
  onClose: () => void;
  onOpenPage: () => void;
}) {
  return (
    <Drawer width={760} title="邮件详情" visible={props.visible} onCancel={props.onClose} footer={<Button type="primary" onClick={props.onOpenPage}>打开详情页</Button>}>
      <MailContent {...props} />
    </Drawer>
  );
}

function MailContent({ message, showCodeTester, codeProjects, openAI, onTestCodeProject, onCopyCode }: {
  message: MailMessage | null;
  showCodeTester?: boolean;
  codeProjects?: CodeProject[];
  openAI?: AdminConfig['openai'];
  onTestCodeProject?: (messageId: string, project: CodeProject, message: TestMessagePayload, useAI?: boolean) => Promise<CodeTestResponse>;
  onCopyCode?: (code: string) => void;
}) {
  if (!message) return <Empty description="选择一封邮件查看内容。" />;
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Typography.Title heading={4}>{message.subject || '无主题'}</Typography.Title>
        <Typography.Text type="secondary">{message.from || 'unknown'} · {formatDate(message.receivedAt)}</Typography.Text>
      </div>
      <Descriptions column={1} border size="small" data={[
        { label: 'Message ID', value: message.id },
        { label: 'From', value: message.from || '-' },
        { label: 'To', value: message.to.join(', ') },
        { label: 'Received', value: formatDate(message.receivedAt) },
      ]} />
      {showCodeTester && onTestCodeProject && onCopyCode && (
        <CodeExtractionTester
          message={message}
          codeProjects={codeProjects || []}
          openAI={openAI}
          onTest={onTestCodeProject}
          onCopyCode={onCopyCode}
        />
      )}
      {message.html ? <iframe title="mail-html" className="mail-frame" srcDoc={message.html} /> : <pre className="mail-text">{message.text || message.raw || '空邮件'}</pre>}
    </Space>
  );
}

function CodeExtractionTester({ message, codeProjects, openAI, onTest, onCopyCode }: {
  message: MailMessage;
  codeProjects: CodeProject[];
  openAI?: AdminConfig['openai'];
  onTest: (messageId: string, project: CodeProject, message: TestMessagePayload, useAI?: boolean) => Promise<CodeTestResponse>;
  onCopyCode: (code: string) => void;
}) {
  const [selectedKey, setSelectedKey] = useState(codeProjects[0]?.id || codeProjects[0]?.name || chatGPTTemplateKey);
  const [testing, setTesting] = useState(false);
  const [aiTesting, setAITesting] = useState(false);
  const [sourceKind, setSourceKind] = useState<'raw' | 'html' | 'text'>(() => preferredEditableSource(codeProjects[0]?.source || chatGPTCodeProject.source, message));
  const [sourceText, setSourceText] = useState(message.raw || message.html || message.text || '');
  const [testPattern, setTestPattern] = useState(codeProjects[0]?.codePattern || chatGPTCodeProject.codePattern);
  const [testSource, setTestSource] = useState<CodeProject['source']>(codeProjects[0]?.source || chatGPTCodeProject.source);
  const [matches, setMatches] = useState<CodeMatch[] | null>(null);
  const [regexSuggestions, setRegexSuggestions] = useState<RegexSuggestion[] | null>(null);
  const [aiError, setAIError] = useState('');
  const selectedProject = useMemo(() => {
    if (selectedKey === chatGPTTemplateKey) return chatGPTCodeProject;
    return codeProjects.find((item) => (item.id || item.name) === selectedKey) || chatGPTCodeProject;
  }, [codeProjects, selectedKey]);

  useEffect(() => {
    setSelectedKey((current) => {
      if (current === chatGPTTemplateKey || codeProjects.some((item) => (item.id || item.name) === current)) return current;
      return codeProjects[0]?.id || codeProjects[0]?.name || chatGPTTemplateKey;
    });
  }, [codeProjects]);

  useEffect(() => {
    setMatches(null);
    setRegexSuggestions(null);
    setAIError('');
    setTestPattern(selectedProject.codePattern);
    setTestSource(selectedProject.source || 'all');
  }, [message.id, selectedKey, selectedProject.codePattern, selectedProject.source]);

  useEffect(() => {
    setSourceKind(preferredEditableSource(testSource, message));
  }, [message.id, testSource, message.raw, message.html, message.text]);

  useEffect(() => {
    const next = sourceKind === 'raw' ? message.raw || '' : sourceKind === 'html' ? message.html || '' : message.text || '';
    setSourceText(next);
  }, [message.id, sourceKind, message.raw, message.html, message.text]);

  const messagePayload = (): TestMessagePayload => ({
    from: message.from,
    to: message.to,
    subject: message.subject,
    text: sourceKind === 'text' ? sourceText : message.text,
    html: sourceKind === 'html' ? sourceText : message.html,
    raw: sourceKind === 'raw' ? sourceText : message.raw || '',
  });

  const projectForTest = useMemo(() => ({
    ...selectedProject,
    codePattern: testPattern,
    source: testSource,
  }), [selectedProject, testPattern, testSource]);

  const runTest = async (suggestRegex = false) => {
    if (suggestRegex) {
      setAITesting(true);
    } else {
      setTesting(true);
    }
    try {
      const result = await onTest(message.id, projectForTest, messagePayload(), suggestRegex);
      if (suggestRegex) {
        setRegexSuggestions(result.regexSuggestions || []);
        setAIError(result.aiError || '');
        if (result.aiError) {
          Message.warning(result.aiError);
        } else if ((result.regexSuggestions || []).length === 0) {
          Message.warning('OpenAI 没有生成可用正则建议');
        } else {
          Message.success(`OpenAI 生成了 ${(result.regexSuggestions || []).length} 条正则建议`);
        }
      } else {
        setMatches(result.matches || []);
        if ((result.matches || []).length === 0) {
          Message.warning('当前项目没有从这封邮件中提取到验证码');
        } else {
          Message.success(`提取到 ${(result.matches || []).length} 个验证码`);
        }
      }
    } catch (error) {
      Message.error((error as Error).message);
    } finally {
      if (suggestRegex) {
        setAITesting(false);
      } else {
        setTesting(false);
      }
    }
  };

  const applySuggestion = (suggestion: RegexSuggestion) => {
    setTestPattern(suggestion.pattern);
    setTestSource(suggestion.source || 'all');
    setSourceKind(preferredEditableSource(suggestion.source || 'all', message));
    setMatches(null);
    Message.success('已套用正则建议，可点击“测试正则提取”验证');
  };

  const copyRegex = async (pattern: string) => {
    await navigator.clipboard?.writeText(pattern);
    Message.success('正则表达式已复制');
  };

  return (
    <Card className="code-test-card" bordered={false} title="测试自动提取验证码" extra={<Tag color="orange">仅测试不保存</Tag>}>
      <Space direction="vertical" size="medium" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          选择验证码项目后，可编辑原始代码并测试当前正则；OpenAI 辅助会生成多条可复用正则建议，不会只提取一次验证码。
        </Typography.Text>
        <Space wrap className="code-test-actions">
          <Select value={selectedKey} onChange={setSelectedKey} style={{ minWidth: 220 }}>
            {codeProjects.map((item) => (
              <Option key={item.id || item.name} value={item.id || item.name}>{item.name}</Option>
            ))}
            <Option value={chatGPTTemplateKey}>ChatGPT 示例（临时）</Option>
          </Select>
          <Button type="primary" icon={<IconThunderbolt />} loading={testing} onClick={() => runTest(false)}>测试正则提取</Button>
          <Button
            icon={<IconRobot />}
            loading={aiTesting}
            disabled={!openAI?.enabled}
            onClick={() => runTest(true)}
          >
            AI 生成正则
          </Button>
          <Tag color={openAI?.enabled && openAI.apiKeySet ? 'green' : 'gray'}>
            OpenAI {openAI?.enabled ? (openAI.apiKeySet ? '已配置' : '缺少 Key') : '未启用'}
          </Tag>
        </Space>
        <Descriptions column={1} size="small" border data={[
          { label: '发件人正则', value: selectedProject.fromPattern || '-' },
          { label: '主题包含', value: selectedProject.subject || '-' },
          { label: '项目原始来源', value: selectedProject.source || 'all' },
          { label: '项目原始正则', value: <span className="code-inline">{selectedProject.codePattern}</span> },
        ]} />
        <div className="code-source-editor">
          <div className="code-source-toolbar">
            <span className="section-strong">本次测试正则</span>
            <Select value={testSource} onChange={setTestSource} size="small" style={{ width: 132 }}>
              <Option value="all">全部内容</Option>
              <Option value="text">Text</Option>
              <Option value="html">HTML</Option>
              <Option value="raw">Raw 原文</Option>
            </Select>
          </div>
          <Input.TextArea
            className="code-source-textarea"
            value={testPattern}
            onChange={setTestPattern}
            autoSize={{ minRows: 2, maxRows: 6 }}
            placeholder="输入或套用 AI 生成的验证码正则。建议使用一个捕获组，例如 code:\\s*(\\d{6})"
          />
        </div>
        <div className="code-source-editor">
          <div className="code-source-toolbar">
            <span className="section-strong">测试原始代码</span>
            <Select value={sourceKind} onChange={setSourceKind} size="small" style={{ width: 132 }}>
              <Option value="raw">Raw 原文</Option>
              <Option value="html">HTML</Option>
              <Option value="text">Text</Option>
            </Select>
          </div>
          <Input.TextArea
            className="code-source-textarea"
            value={sourceText}
            onChange={setSourceText}
            autoSize={{ minRows: 8, maxRows: 18 }}
            placeholder="这里显示并允许编辑当前邮件原始内容，测试时会使用编辑后的内容。"
          />
        </div>
        {matches && (
          matches.length === 0 ? (
            <Empty description="没有提取到验证码，可调整项目正则后再测试。" />
          ) : (
            <div className="code-result-scroll">
              {matches.map((item) => (
                <div className="code-test-result" key={item.id}>
                  <div>
                    <Tag color="green">{item.projectName}</Tag>
                    <span className="code-inline code-result-value">{item.code}</span>
                  </div>
                  <Button size="small" icon={<IconCopy />} onClick={() => onCopyCode(item.code)}>复制</Button>
                </div>
              ))}
            </div>
          )
        )}
        {regexSuggestions && (
          <RegexSuggestionList
            suggestions={regexSuggestions}
            onApply={applySuggestion}
            onCopy={copyRegex}
          />
        )}
        {aiError && <Typography.Text type="error">{aiError}</Typography.Text>}
      </Space>
    </Card>
  );
}

function RegexSuggestionList({ suggestions, onApply, onCopy }: {
  suggestions: RegexSuggestion[];
  onApply: (suggestion: RegexSuggestion) => void;
  onCopy: (pattern: string) => void;
}) {
  return (
    <Space direction="vertical" size="small" style={{ width: '100%' }}>
      <span className="section-strong">AI 正则建议</span>
      {suggestions.length === 0 ? (
        <Empty description="OpenAI 没有生成正则建议。" />
      ) : (
        <div className="regex-suggestion-scroll">
          {suggestions.map((item, index) => (
            <div className="regex-suggestion" key={`${item.pattern}-${index}`}>
              <div className="regex-suggestion-main">
                <Space wrap size="small">
                  <Tag color="arcoblue">{item.name || `建议 ${index + 1}`}</Tag>
                  <Tag>{item.source || 'all'}</Tag>
                  {item.sampleCode && <Tag color="green">样例 {item.sampleCode}</Tag>}
                  <Tag color={item.confidence >= 0.8 ? 'green' : 'orange'}>{Math.round((item.confidence || 0) * 100)}%</Tag>
                </Space>
                <div className="code-inline regex-pattern">{item.pattern}</div>
                {item.reason && <Typography.Text type="secondary">{item.reason}</Typography.Text>}
              </div>
              <Space>
                <Button size="small" onClick={() => onApply(item)}>套用测试</Button>
                <Button size="small" icon={<IconCopy />} onClick={() => onCopy(item.pattern)}>复制正则</Button>
              </Space>
            </div>
          ))}
        </div>
      )}
    </Space>
  );
}

function preferredEditableSource(source: CodeProject['source'] | undefined, message: MailMessage): 'raw' | 'html' | 'text' {
  if (source === 'text' || source === 'html' || source === 'raw') return source;
  if (message.raw) return 'raw';
  if (message.html) return 'html';
  return 'text';
}

function CodeTestResultList({ title, matches, empty, onCopyCode }: {
  title: string;
  matches: CodeMatch[];
  empty: string;
  onCopyCode: (code: string) => void;
}) {
  return (
    <Space direction="vertical" size="small" style={{ width: '100%' }}>
      <span className="section-strong">{title}</span>
      {matches.length === 0 ? (
        <Empty description={empty} />
      ) : (
        <div className="code-result-scroll">
          {matches.map((item) => (
            <div className="code-test-result" key={`${title}-${item.code}-${item.mailbox}`}>
              <div>
                <Tag color="green">{item.projectName}</Tag>
                <span className="code-inline code-result-value">{item.code}</span>
              </div>
              <Button size="small" icon={<IconCopy />} onClick={() => onCopyCode(item.code)}>复制</Button>
            </div>
          ))}
        </div>
      )}
    </Space>
  );
}

function AdminGuard(props: {
  adminReady: boolean;
  publicConfig: PublicConfig;
  authForm: AdminAuthForm;
  onAuthFormChange: (value: AdminAuthForm) => void;
  onSubmit: () => void;
  children: ReactNode;
}) {
  if (props.adminReady) return props.children;
  return <AdminAuthCard publicConfig={props.publicConfig} authForm={props.authForm} onAuthFormChange={props.onAuthFormChange} onSubmit={props.onSubmit} />;
}

function AdminAuthCard({ publicConfig, authForm, onAuthFormChange, onSubmit, onBack }: {
  publicConfig: PublicConfig;
  authForm: AdminAuthForm;
  onAuthFormChange: (value: AdminAuthForm) => void;
  onSubmit: () => void;
  onBack?: () => void;
}) {
  return (
    <Card className="console-card auth-panel" bordered={false}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <div className="auth-title"><IconUser /> {publicConfig.adminSetupRequired ? '初始化管理员' : '管理员登录'}</div>
        <Form layout="vertical">
          <Form.Item label="用户名">
            <Input value={authForm.username} onChange={(username) => onAuthFormChange({ ...authForm, username })} />
          </Form.Item>
          <Form.Item label="管理员密码">
            <Input.Password value={authForm.password} onChange={(password) => onAuthFormChange({ ...authForm, password })} onPressEnter={onSubmit} />
          </Form.Item>
          {publicConfig.adminSetupRequired && (
            <Form.Item label="私有访问密码（可选）">
              <Input.Password value={authForm.accessPassword} onChange={(accessPassword) => onAuthFormChange({ ...authForm, accessPassword })} />
            </Form.Item>
          )}
          <Space direction="vertical" style={{ width: '100%' }}>
            <Button type="primary" long onClick={onSubmit}>{publicConfig.adminSetupRequired ? '初始化' : '登录'}</Button>
            {onBack && <Button long onClick={onBack}>返回收件台</Button>}
          </Space>
        </Form>
      </Space>
    </Card>
  );
}

function AdminOverview({ publicConfig, adminConfig, adminMessages, adminCodes, onRefresh }: {
  publicConfig: PublicConfig;
  adminConfig: AdminConfig | null;
  adminMessages: MailMessage[];
  adminCodes: CodeMatch[];
  onRefresh: () => void;
}) {
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}><MetricCard title="全部邮件" value={adminMessages.length} icon={<IconArchive />} /></Col>
        <Col xs={24} sm={12} lg={6}><MetricCard title="验证码项目" value={adminConfig?.codeProjects.length || 0} icon={<IconRobot />} /></Col>
        <Col xs={24} sm={12} lg={6}><MetricCard title="已提取验证码" value={adminCodes.length} icon={<IconThunderbolt />} /></Col>
        <Col xs={24} sm={12} lg={6}><MetricCard title="域名数量" value={publicConfig.domains.length} icon={<IconApps />} /></Col>
      </Row>
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={14}>
          <Card className="console-card" title="运行状态" bordered={false} extra={<Button icon={<IconRefresh />} onClick={onRefresh}>刷新</Button>}>
            <Descriptions column={1} border data={[
              { label: 'HTTP 地址', value: adminConfig?.server.httpAddr || '-' },
              { label: 'SMTP 地址', value: adminConfig?.smtp.addr || '-' },
              { label: '配置文件', value: adminConfig?.configPath || '-' },
              { label: '邮件保留时间', value: adminConfig?.mail.retention || '-' },
              { label: '管理员', value: adminConfig?.admin.username || '-' },
              { label: '访问密码', value: adminConfig?.access.passwordSet ? '已设置' : '未设置' },
              { label: '外部 API', value: adminConfig?.api.enabled ? (adminConfig.api.tokenSet ? '启用 · Token 已设置' : '启用 · 缺少 Token') : '未启用' },
              { label: 'OpenAI 正则', value: adminConfig?.openai.enabled ? `启用 · ${adminConfig.openai.model} · ${adminConfig.openai.apiMode}` : '未启用' },
              { label: 'WebHook 规则', value: adminConfig?.webhooks.length || 0 },
            ]} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card className="console-card" title="域名内容" bordered={false}>
            <Space wrap>
              {(adminConfig?.mail.domains || publicConfig.domains).map((item) => <Tag key={item} color="arcoblue">{item}</Tag>)}
            </Space>
          </Card>
        </Col>
      </Row>
    </Space>
  );
}

function SettingsPage({ adminConfig, draft, setDraft, onSave }: {
  adminConfig: AdminConfig | null;
  draft: ConfigDraft | null;
  setDraft: (value: ConfigDraft) => void;
  onSave: () => void;
}) {
  if (!draft) return <Card className="console-card" bordered={false}><Empty description="配置加载中" /></Card>;
  return (
    <Card className="console-card" title="系统配置" bordered={false} extra={<Button type="primary" icon={<IconSettings />} onClick={onSave}>保存配置</Button>}>
      <Form layout="vertical" className="settings-form">
        <Descriptions column={1} border size="small" className="config-path-box" data={[
          { label: '当前配置文件', value: adminConfig?.configPath || 'config/config.json' },
        ]} />
        <Row gutter={16}>
          <Col xs={24} md={12}>
            <Form.Item label="HTTP 监听地址"><Input value={draft.httpAddr} onChange={(httpAddr) => setDraft({ ...draft, httpAddr })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="SMTP 监听地址"><Input value={draft.smtpAddr} onChange={(smtpAddr) => setDraft({ ...draft, smtpAddr })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="邮件保留时间"><Input value={draft.retention} onChange={(retention) => setDraft({ ...draft, retention })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="访问模式">
              <Select value={draft.accessMode} onChange={(accessMode) => setDraft({ ...draft, accessMode })}>
                <Option value="public">公开模式</Option>
                <Option value="private">私有模式</Option>
              </Select>
            </Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="收信域名（每行一个）"><Input.TextArea autoSize={{ minRows: 4 }} value={draft.domains} onChange={(domains) => setDraft({ ...draft, domains })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="保留邮箱前缀（每行一个）"><Input.TextArea autoSize={{ minRows: 4 }} value={draft.reservedLocalParts} onChange={(reservedLocalParts) => setDraft({ ...draft, reservedLocalParts })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="访问密码"><Input.Password placeholder={adminConfig?.access.passwordSet ? '已设置，留空不修改' : '设置访问密码'} value={draft.accessPassword} onChange={(accessPassword) => setDraft({ ...draft, accessPassword })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="管理员用户名"><Input value={draft.adminUsername} onChange={(adminUsername) => setDraft({ ...draft, adminUsername })} /></Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label="管理员密码"><Input.Password placeholder={adminConfig?.admin.passwordSet ? '已设置，留空不修改' : '设置管理员密码'} value={draft.adminPassword} onChange={(adminPassword) => setDraft({ ...draft, adminPassword })} /></Form.Item>
          </Col>
        </Row>
        <Card className="settings-subcard" title="外部 API 调用" bordered={false}>
          <Row gutter={16}>
            <Col xs={24} md={12}>
              <Form.Item label="启用 API">
                <Switch checked={draft.apiEnabled} checkedText="启用" uncheckedText="停用" onChange={(apiEnabled) => setDraft({ ...draft, apiEnabled })} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="API Token">
                <Input.Password
                  placeholder={adminConfig?.api.tokenSet ? '已设置，留空不修改' : '至少 12 个字符'}
                  value={draft.apiToken}
                  onChange={(apiToken) => setDraft({ ...draft, apiToken, apiClearToken: false })}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="Token 状态">
                <Space wrap>
                  <Tag color={adminConfig?.api.tokenSet ? 'green' : 'gray'}>{adminConfig?.api.tokenSet ? '已保存' : '未保存'}</Tag>
                  <Button size="small" disabled={!adminConfig?.api.tokenSet} onClick={() => setDraft({ ...draft, apiToken: '', apiClearToken: true })}>保存时清除 Token</Button>
                </Space>
              </Form.Item>
            </Col>
          </Row>
          <Typography.Text type="secondary">外部软件可使用 `Authorization: Bearer &lt;Token&gt;` 调用 `/api/v1`，Token 只保存 hash，不会在管理接口中回显。</Typography.Text>
        </Card>
        <Card className="settings-subcard" title="OpenAI 正则生成" bordered={false}>
          <Row gutter={16}>
            <Col xs={24} md={12}>
              <Form.Item label="启用 OpenAI">
                <Switch checked={draft.openAIEnabled} checkedText="启用" uncheckedText="停用" onChange={(openAIEnabled) => setDraft({ ...draft, openAIEnabled })} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="API Key">
                <Input.Password
                  placeholder={adminConfig?.openai.apiKeySet ? '已设置，留空不修改' : 'sk-...'}
                  value={draft.openAIAPIKey}
                  onChange={(openAIAPIKey) => setDraft({ ...draft, openAIAPIKey, openAIClearAPIKey: false })}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="Base URL"><Input value={draft.openAIBaseURL} onChange={(openAIBaseURL) => setDraft({ ...draft, openAIBaseURL })} /></Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="模型"><Input value={draft.openAIModel} onChange={(openAIModel) => setDraft({ ...draft, openAIModel })} /></Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="API 模式">
                <Select value={draft.openAIAPIMode} onChange={(openAIAPIMode) => setDraft({ ...draft, openAIAPIMode })}>
                  <Option value="auto">自动：Responses 优先，失败时降级 Chat Completions</Option>
                  <Option value="responses">仅 Responses API</Option>
                  <Option value="chat_completions">仅 Chat Completions API</Option>
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="超时时间"><Input value={draft.openAITimeout} onChange={(openAITimeout) => setDraft({ ...draft, openAITimeout })} /></Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="Key 状态">
                <Space wrap>
                  <Tag color={adminConfig?.openai.apiKeySet ? 'green' : 'gray'}>{adminConfig?.openai.apiKeySet ? '已保存' : '未保存'}</Tag>
                  <Button size="small" disabled={!adminConfig?.openai.apiKeySet} onClick={() => setDraft({ ...draft, openAIAPIKey: '', openAIClearAPIKey: true })}>保存时清除 Key</Button>
                </Space>
              </Form.Item>
            </Col>
          </Row>
          <Typography.Text type="secondary">API Key 只写入服务端 JSON 配置，不会在管理接口中回显；第三方兼容网关不支持 `/responses` 时，可选择 Chat Completions 模式。</Typography.Text>
        </Card>
      </Form>
    </Card>
  );
}

function CodeProjectsPage({ adminConfig, codes, onCreate, onCreateChatGPT, onEdit, onDelete, onCopyCode }: {
  adminConfig: AdminConfig | null;
  codes: CodeMatch[];
  onCreate: () => void;
  onCreateChatGPT: () => void;
  onEdit: (project: CodeProject) => void;
  onDelete: (id?: string) => void;
  onCopyCode: (code: string) => void;
}) {
  const projectColumns = [
    { title: '项目名称', dataIndex: 'name' },
    { title: '发件人正则', dataIndex: 'fromPattern', ellipsis: true, render: (value: string) => value || '-' },
    { title: '验证码正则', dataIndex: 'codePattern', ellipsis: true },
    { title: '来源', dataIndex: 'source', render: (value: string) => <Tag>{value || 'all'}</Tag> },
    { title: '状态', render: (_: unknown, item: CodeProject) => <Tag color={item.enabled ? 'green' : 'gray'}>{item.enabled ? '启用' : '停用'}</Tag> },
    {
      title: '操作',
      width: 180,
      render: (_: unknown, item: CodeProject) => (
        <Space>
          <Button size="small" icon={<IconEdit />} onClick={() => onEdit(item)}>编辑</Button>
          <Button size="small" status="danger" icon={<IconDelete />} onClick={() => onDelete(item.id)}>删除</Button>
        </Space>
      ),
    },
  ];
  const codeColumns = [
    { title: '项目', dataIndex: 'projectName' },
    { title: '验证码', render: (_: unknown, item: CodeMatch) => <span className="code-inline">{item.code}</span> },
    { title: '邮箱', dataIndex: 'mailbox', ellipsis: true },
    { title: '发件人', dataIndex: 'from', ellipsis: true },
    { title: '时间', render: (_: unknown, item: CodeMatch) => formatDate(item.receivedAt) },
    { title: '操作', width: 100, render: (_: unknown, item: CodeMatch) => <Button size="small" icon={<IconCopy />} onClick={() => onCopyCode(item.code)}>复制</Button> },
  ];
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card
        className="console-card"
        title="验证码项目"
        bordered={false}
        extra={(
          <Space wrap>
            <Button icon={<IconRobot />} onClick={onCreateChatGPT}>ChatGPT 示例</Button>
            <Button type="primary" icon={<IconPlus />} onClick={onCreate}>新建项目</Button>
          </Space>
        )}
      >
        <Table
          rowKey="id"
          data={adminConfig?.codeProjects || []}
          columns={projectColumns}
          pagination={false}
          noDataElement={<Empty description="暂无验证码项目，可先创建 ChatGPT 示例。" />}
        />
      </Card>
      <Card className="console-card" title="最近提取结果" bordered={false}>
        <Table
          rowKey="id"
          data={codes}
          columns={codeColumns}
          pagination={{ pageSize: 8 }}
          noDataElement={<Empty description="暂未提取到验证码。" />}
        />
      </Card>
    </Space>
  );
}

function CodeProjectModal({ visible, project, editing, onChange, onCancel, onSave }: {
  visible: boolean;
  project: CodeProject;
  editing: boolean;
  onChange: (project: CodeProject) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  return (
    <Modal title={editing ? '编辑验证码项目' : '新建验证码项目'} visible={visible} onCancel={onCancel} onOk={onSave} okText="保存" style={{ width: 620 }}>
      <Form layout="vertical">
        <Form.Item label="项目名称"><Input placeholder="ChatGPT 注册" value={project.name} onChange={(name) => onChange({ ...project, name })} /></Form.Item>
        <Form.Item label="项目说明"><Input.TextArea autoSize={{ minRows: 2 }} value={project.description} onChange={(description) => onChange({ ...project, description })} /></Form.Item>
        <Form.Item label="发件人正则">
          <Input placeholder="(?i)openai\\.com" value={project.fromPattern} onChange={(fromPattern) => onChange({ ...project, fromPattern })} />
        </Form.Item>
        <Form.Item label="验证码正则">
          <Input value={project.codePattern} onChange={(codePattern) => onChange({ ...project, codePattern })} />
          <Typography.Text type="secondary">包含捕获组时会使用第一个非空捕获组，例如 ChatGPT HTML：<span className="code-inline">{'(?is)<h1[^>]*>\\s*(\\d{6})\\s*</h1>|enter this code:\\s*(\\d{6})'}</span></Typography.Text>
        </Form.Item>
        <Form.Item label="主题包含"><Input value={project.subject} onChange={(subject) => onChange({ ...project, subject })} /></Form.Item>
        <Form.Item label="限定收信域名（逗号或换行分隔）"><Input value={(project.domains || []).join(',')} onChange={(value) => onChange({ ...project, domains: splitList(value) })} /></Form.Item>
        <Form.Item label="限定邮箱前缀（逗号或换行分隔）"><Input value={(project.localParts || []).join(',')} onChange={(value) => onChange({ ...project, localParts: splitList(value) })} /></Form.Item>
        <Form.Item label="提取来源">
          <Select value={project.source} onChange={(source) => onChange({ ...project, source })}>
            <Option value="all">全部内容</Option>
            <Option value="text">纯文本</Option>
            <Option value="html">HTML</Option>
            <Option value="raw">原文</Option>
          </Select>
        </Form.Item>
        <Form.Item label="启用状态"><Switch checked={project.enabled} checkedText="启用" uncheckedText="停用" onChange={(enabled) => onChange({ ...project, enabled })} /></Form.Item>
      </Form>
    </Modal>
  );
}

function WebhooksPage({ adminConfig, onCreate, onEdit, onDelete }: {
  adminConfig: AdminConfig | null;
  onCreate: () => void;
  onEdit: (rule: WebhookRule) => void;
  onDelete: (id?: string) => void;
}) {
  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '目标 URL', dataIndex: 'url', ellipsis: true },
    { title: '提取来源', dataIndex: 'source', render: (value: string) => <Tag>{value || 'all'}</Tag> },
    { title: '状态', render: (_: unknown, item: WebhookRule) => <Tag color={item.enabled ? 'green' : 'gray'}>{item.enabled ? '启用' : '停用'}</Tag> },
    {
      title: '操作',
      width: 180,
      render: (_: unknown, item: WebhookRule) => (
        <Space>
          <Button size="small" icon={<IconEdit />} onClick={() => onEdit(item)}>编辑</Button>
          <Button size="small" status="danger" icon={<IconDelete />} onClick={() => onDelete(item.id)}>删除</Button>
        </Space>
      ),
    },
  ];
  return (
    <Card className="console-card" title="WebHook 规则" bordered={false} extra={<Button type="primary" icon={<IconPlus />} onClick={onCreate}>新建规则</Button>}>
      <Table rowKey="id" data={adminConfig?.webhooks || []} columns={columns} pagination={false} noDataElement={<Empty description="暂无 WebHook 规则" />} />
    </Card>
  );
}

function WebhookModal({ visible, rule, editing, onChange, onCancel, onSave }: {
  visible: boolean;
  rule: WebhookRule;
  editing: boolean;
  onChange: (rule: WebhookRule) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  return (
    <Modal title={editing ? '编辑 WebHook 规则' : '新建 WebHook 规则'} visible={visible} onCancel={onCancel} onOk={onSave} okText="保存">
      <Form layout="vertical">
        <Form.Item label="规则名称"><Input value={rule.name} onChange={(name) => onChange({ ...rule, name })} /></Form.Item>
        <Form.Item label="WebHook URL"><Input value={rule.url} onChange={(url) => onChange({ ...rule, url })} /></Form.Item>
        <Form.Item label="验证码正则"><Input value={rule.codePattern} onChange={(codePattern) => onChange({ ...rule, codePattern })} /></Form.Item>
        <Form.Item label="主题包含"><Input value={rule.subject} onChange={(subject) => onChange({ ...rule, subject })} /></Form.Item>
        <Form.Item label="发件人正则"><Input value={rule.fromPattern} onChange={(fromPattern) => onChange({ ...rule, fromPattern })} /></Form.Item>
        <Form.Item label="限定域名（逗号分隔）"><Input value={(rule.domains || []).join(',')} onChange={(value) => onChange({ ...rule, domains: splitList(value) })} /></Form.Item>
        <Form.Item label="限定邮箱前缀（逗号分隔）"><Input value={(rule.localParts || []).join(',')} onChange={(value) => onChange({ ...rule, localParts: splitList(value) })} /></Form.Item>
        <Form.Item label="提取来源">
          <Select value={rule.source} onChange={(source) => onChange({ ...rule, source })}>
            <Option value="all">全部内容</Option>
            <Option value="text">纯文本</Option>
            <Option value="html">HTML</Option>
            <Option value="raw">原文</Option>
          </Select>
        </Form.Item>
        <Form.Item label="启用状态"><Switch checked={rule.enabled} checkedText="启用" uncheckedText="停用" onChange={(enabled) => onChange({ ...rule, enabled })} /></Form.Item>
      </Form>
    </Modal>
  );
}

function DeliveryDebugPage({ events, allEvents, dnsReport, loading, mailboxFilter, typeFilter, onMailboxFilterChange, onTypeFilterChange, onRefresh, onClear }: {
  events: SMTPDebugEvent[];
  allEvents: SMTPDebugEvent[];
  dnsReport: DNSDebugReport | null;
  loading: boolean;
  mailboxFilter: string;
  typeFilter: string;
  onMailboxFilterChange: (value: string) => void;
  onTypeFilterChange: (value: string) => void;
  onRefresh: () => void;
  onClear: () => void;
}) {
  const typeOptions = Array.from(new Set(allEvents.map((item) => item.type).filter(Boolean))).sort();
  const copyReport = async () => {
    await navigator.clipboard?.writeText(JSON.stringify({ dnsReport, events: allEvents }, null, 2));
    Message.success('投递调试结果已复制');
  };
  const columns = [
    { title: '时间', width: 170, render: (_: unknown, item: SMTPDebugEvent) => formatDate(item.time) },
    { title: '事件', width: 130, render: (_: unknown, item: SMTPDebugEvent) => <Tag color={smtpEventColor(item.type)}>{smtpEventLabel(item.type)}</Tag> },
    { title: '来源', dataIndex: 'remoteAddr', ellipsis: true, render: (value: string) => value || '-' },
    { title: 'HELO/EHLO', dataIndex: 'helo', ellipsis: true, render: (value: string) => value || '-' },
    { title: '发件人', dataIndex: 'from', ellipsis: true, render: (value: string) => value || '-' },
    {
      title: '收件人',
      ellipsis: true,
      render: (_: unknown, item: SMTPDebugEvent) => item.to || item.recipients?.join(', ') || '-',
    },
    { title: '消息/大小', width: 150, render: (_: unknown, item: SMTPDebugEvent) => item.messageId ? `${item.messageId.slice(0, 10)} · ${formatBytes(item.size)}` : formatBytes(item.size) },
    { title: '错误/说明', ellipsis: true, render: (_: unknown, item: SMTPDebugEvent) => item.error || item.detail || '-' },
  ];
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card className="console-card" title="DNS 与监听诊断" bordered={false} extra={<Button icon={<IconRefresh />} loading={loading} onClick={onRefresh}>刷新诊断</Button>}>
        <DNSDebugSummary report={dnsReport} />
      </Card>
      <Card
        className="console-card"
        title="SMTP 投递事件"
        bordered={false}
        extra={(
          <Space wrap>
            <Button icon={<IconCopy />} disabled={!dnsReport && allEvents.length === 0} onClick={copyReport}>复制诊断</Button>
            <Button icon={<IconRefresh />} loading={loading} onClick={onRefresh}>刷新</Button>
            <Button status="danger" icon={<IconDelete />} disabled={allEvents.length === 0} onClick={onClear}>清空</Button>
          </Space>
        )}
      >
        <Space className="delivery-debug-filters" wrap>
          <Input
            allowClear
            prefix={<IconSearch />}
            placeholder="筛选邮箱、发件人或 IP"
            value={mailboxFilter}
            onChange={onMailboxFilterChange}
          />
          <Select allowClear placeholder="事件类型" value={typeFilter || undefined} onChange={(value) => onTypeFilterChange(value || '')} style={{ width: 180 }}>
            {typeOptions.map((type) => <Option key={type} value={type}>{smtpEventLabel(type)}</Option>)}
          </Select>
        </Space>
        <Table
          rowKey="id"
          loading={loading}
          data={events}
          columns={columns}
          pagination={{ pageSize: 12 }}
          noDataElement={<Empty description="暂无 SMTP 投递事件。请求验证码后，如果这里没有连接记录，问题通常在 DNS、OpenAI 投递或公网 25 端口之前。" />}
        />
      </Card>
    </Space>
  );
}

function DNSDebugSummary({ report }: { report: DNSDebugReport | null }) {
  if (!report) return <Empty description="点击刷新诊断，检查当前配置域名的 MX/A/AAAA 和 SMTP 监听端口。" />;
  const allIssues = [...(report.issues || []), ...report.domains.flatMap((item) => item.issues || [])];
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Descriptions column={1} border size="small" data={[
        { label: '检查时间', value: formatDate(report.checkedAt) },
        { label: 'SMTP 监听', value: <Space><Tag color={report.smtpPort === '25' ? 'green' : 'orange'}>{report.smtpAddr || '-'}</Tag><Typography.Text type="secondary">公网收信建议使用 25 端口</Typography.Text></Space> },
      ]} />
      {allIssues.length > 0 ? (
        <Space direction="vertical" size="small" style={{ width: '100%' }}>
          <span className="section-strong">发现的问题</span>
          {allIssues.map((issue, index) => (
            <div className="debug-issue" key={`${issue.level}-${index}`}>
              <Tag color={issue.level === 'error' ? 'red' : 'orange'}>{issue.level === 'error' ? '错误' : '警告'}</Tag>
              <span>{issue.message}</span>
            </div>
          ))}
        </Space>
      ) : (
        <Tag color="green">未发现明显 DNS 风险</Tag>
      )}
      {report.domains.map((domain) => (
        <Card className="settings-subcard" title={domain.domain} bordered={false} key={domain.domain}>
          <Space direction="vertical" size="small" style={{ width: '100%' }}>
            <div>
              <span className="section-strong">域名 A/AAAA：</span>
              <AddressTags addresses={domain.addresses || []} />
            </div>
            <Table
              rowKey="host"
              data={domain.mx || []}
              columns={[
                { title: 'MX 主机', dataIndex: 'host', render: (value: string, item: DNSMX) => <Space><span>{value}</span>{item.isIP && <Tag color="red">直接 IP</Tag>}</Space> },
                { title: '优先级', dataIndex: 'preference', width: 100 },
                { title: 'A/AAAA', render: (_: unknown, item: DNSMX) => <AddressTags addresses={item.addresses || []} /> },
                { title: '错误', dataIndex: 'error', render: (value: string) => value || '-' },
              ]}
              pagination={false}
              noDataElement={<Empty description="没有 MX 记录" />}
            />
          </Space>
        </Card>
      ))}
    </Space>
  );
}

function AddressTags({ addresses }: { addresses: DNSAddress[] }) {
  if (addresses.length === 0) return <Typography.Text type="secondary">无</Typography.Text>;
  return (
    <Space wrap>
      {addresses.map((address) => (
        <Tag key={`${address.version}-${address.ip}`} color={address.flags?.length ? 'orange' : 'arcoblue'}>
          {address.version} {address.ip}{address.flags?.length ? ` · ${address.flags.join(',')}` : ''}
        </Tag>
      ))}
    </Space>
  );
}

function AllMessagesPage({ messages, loading, onOpenMessage, onRefresh }: {
  messages: MailMessage[];
  loading: boolean;
  onOpenMessage: (message: MailMessage, preferPage?: boolean, mode?: DetailMode) => void;
  onRefresh: () => void;
}) {
  const columns = [
    { title: '主题', dataIndex: 'subject', render: (value: string) => value || '无主题' },
    { title: '发件人', dataIndex: 'from', ellipsis: true },
    { title: '收件人', render: (_: unknown, item: MailMessage) => item.to.join(', ') },
    { title: '时间', render: (_: unknown, item: MailMessage) => formatDate(item.receivedAt) },
    { title: '操作', width: 100, render: (_: unknown, item: MailMessage) => <Button size="small" type="text" onClick={() => onOpenMessage(item, false, 'admin')}>查看</Button> },
  ];
  return (
    <Card className="console-card" title="全部邮件" bordered={false} extra={<Button icon={<IconRefresh />} onClick={onRefresh}>刷新</Button>}>
      <Table
        rowKey="id"
        loading={loading}
        data={messages}
        columns={columns}
        pagination={{ pageSize: 10 }}
        noDataElement={<Empty description="内存中暂无邮件" />}
        onRow={(record) => ({ onClick: () => onOpenMessage(record as MailMessage, false, 'admin') })}
      />
    </Card>
  );
}

export default App;


