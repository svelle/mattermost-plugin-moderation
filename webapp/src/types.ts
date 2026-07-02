export type ReportStatus = 'open' | 'escalated' | 'resolved';

export type Report = {
    id: string;
    channel_id: string;
    post_id?: string;
    target_user_id: string;
    reporter_user_id: string;
    reason: string;
    note?: string;
    excerpt?: string;
    status: ReportStatus;
    resolution?: string;
    resolved_by_id?: string;
    create_at: number;
    update_at: number;
    target_username: string;
    target_display_name: string;
    reporter_name: string;
    resolved_by_name?: string;
};

export type ChannelModerationStatus = {
    is_moderator: boolean;
    is_admin: boolean;
    open_report_count: number;
};

export type MemberInfo = {
    user_id: string;
    username: string;
    display_name: string;
    is_moderator: boolean;
    is_admin: boolean;
    is_banned: boolean;
    muted: boolean;
    timeout_until?: number;
    timeout_label?: string;
};

export type ReportModalState = {
    channelId: string;
    targetUserId: string;
    targetName: string;
    postId?: string;
    excerpt?: string;
};

export type TimeoutModalState = {
    channelId: string;
    targetUserId: string;
    targetName: string;
    reportId?: string;
};

export type ConfirmAction =
    | {kind: 'delete_post'; channelId: string; postId: string; reportId?: string}
    | {kind: 'ban'; channelId: string; targetUserId: string; targetName: string; reportId?: string}
    | {kind: 'remove'; channelId: string; targetUserId: string; targetName: string};

export type ConfirmModalState = {
    icon: string;
    title: string;
    body: string;
    confirmLabel: string;
    action: ConfirmAction;
};

export type ToastState = {
    message: string;
    icon: string;
};

export type ModerationState = {
    statuses: Record<string, ChannelModerationStatus>;
    reports: Record<string, Report[]>;
    members: Record<string, MemberInfo[]>;
    rhsTab: 'reports' | 'members';
    reportModal: ReportModalState | null;
    timeoutModal: TimeoutModalState | null;
    confirmModal: ConfirmModalState | null;
    toast: ToastState | null;
};

export const REPORT_REASONS = [
    {id: 'spam', label: 'Spam or scam', desc: 'Unsolicited links, phishing, crypto/airdrop scams'},
    {id: 'harass', label: 'Harassment or abuse', desc: 'Targeted insults, threats, or hate speech'},
    {id: 'imp', label: 'Impersonation', desc: 'Pretending to be staff, IT, or another member'},
    {id: 'nsfw', label: 'Inappropriate content', desc: 'Explicit, graphic, or clearly off-topic material'},
    {id: 'other', label: 'Something else', desc: 'Describe the issue in the note below'},
];

export const TIMEOUT_DURATIONS = [
    {id: '5m', label: '5 minutes'},
    {id: '1h', label: '1 hour'},
    {id: '24h', label: '24 hours'},
    {id: '7d', label: '7 days'},
];
