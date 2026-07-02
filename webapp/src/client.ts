import manifest from 'manifest';

import type {ChannelModerationStatus, MemberInfo, Report} from './types';

// basePath is the subpath Mattermost is served from (empty for root),
// derived from the server's SiteURL at plugin initialization.
let basePath = '';

export const setBasePath = (siteURL?: string) => {
    if (!siteURL) {
        return;
    }
    try {
        basePath = new URL(siteURL).pathname.replace(/\/+$/, '');
    } catch {
        // Ignore malformed SiteURL and keep the root path.
    }
};

export const getBasePath = () => basePath;

const apiUrl = () => `${basePath}/plugins/${manifest.id}/api/v1`;

const csrfToken = (): string => {
    const match = document.cookie.match(/(?:^|;\s*)MMCSRF=([^;]+)/);
    return match ? match[1] : '';
};

const doFetch = async <T>(url: string, options: {method?: string; body?: string} = {}): Promise<T> => {
    const method = options.method || 'GET';
    const headers: Record<string, string> = {
        'X-Requested-With': 'XMLHttpRequest',
    };
    if (method !== 'GET') {
        headers['Content-Type'] = 'application/json';
        headers['X-CSRF-Token'] = csrfToken();
    }
    const response = await fetch(url, {
        method,
        body: options.body,
        headers,
        credentials: 'same-origin',
    });
    let data: Record<string, unknown> = {};
    try {
        data = await response.json();
    } catch {
        // Non-JSON response body.
    }
    if (!response.ok) {
        throw new Error(String(data.message || data.error || 'Something went wrong. Please try again.'));
    }
    return data as unknown as T;
};

const doPost = <T>(url: string, body: Record<string, unknown>): Promise<T> => {
    return doFetch<T>(url, {method: 'POST', body: JSON.stringify(body)});
};

export const fetchStatus = (channelId: string) => {
    return doFetch<ChannelModerationStatus>(`${apiUrl()}/channel/${channelId}/status`);
};

export const fetchReports = (channelId: string) => {
    return doFetch<Report[]>(`${apiUrl()}/channel/${channelId}/reports`);
};

export const fetchMembers = (channelId: string, term?: string) => {
    const query = term ? `?q=${encodeURIComponent(term)}` : '';
    return doFetch<MemberInfo[]>(`${apiUrl()}/channel/${channelId}/members${query}`);
};

export const createReport = (report: {channel_id: string; post_id?: string; target_user_id: string; reason: string; note: string}) => {
    return doPost(`${apiUrl()}/reports`, report);
};

export const dismissReport = (channelId: string, reportId: string) => {
    return doPost(`${apiUrl()}/reports/${reportId}/dismiss`, {channel_id: channelId});
};

type MemberAction = {
    channel_id?: string;
    user_id?: string;
    post_id?: string;
    report_id?: string;
    duration?: string;
    moderator?: boolean;
};

export const memberAction = (action: string, body: MemberAction) => {
    return doPost<Record<string, string>>(`${apiUrl()}/actions/${action}`, body);
};

export const setModerator = (channelId: string, userId: string, moderator: boolean) => {
    return doPost(`${apiUrl()}/channel/${channelId}/moderators`, {user_id: userId, moderator});
};

export const errorMessage = (err: unknown): string => {
    if (err instanceof Error && err.message) {
        return err.message;
    }
    return 'Something went wrong. Please try again.';
};
