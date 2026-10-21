import manifest from 'manifest';

import type {GlobalState} from '@mattermost/types/store';

import type {ChannelModerationStatus, ModerationState} from './types';

const emptyStatus: ChannelModerationStatus = {
    is_moderator: false,
    is_admin: false,
    open_report_count: 0,
};

type StateWithPlugin = GlobalState & {
    [key: string]: unknown;
};

// The slice can be absent before the reducer registers, so the return type
// is honest about nullability — callers use optional chaining.
export const pluginState = (state: GlobalState): ModerationState | undefined => {
    return (state as StateWithPlugin)[`plugins-${manifest.id}`] as ModerationState | undefined;
};

export const getCurrentChannelId = (state: GlobalState): string => {
    return state.entities.channels.currentChannelId;
};

export const getCurrentUserId = (state: GlobalState): string => {
    return state.entities.users.currentUserId;
};

export const getStatusForChannel = (state: GlobalState, channelId: string): ChannelModerationStatus => {
    return pluginState(state)?.statuses[channelId] || emptyStatus;
};

export const getStatusForCurrentChannel = (state: GlobalState): ChannelModerationStatus => {
    return getStatusForChannel(state, getCurrentChannelId(state));
};

const plural = (n: number, unit: string) => (n === 1 ? `1 ${unit}` : `${n} ${unit}s`);

// remainingLabel mirrors the server's wording for how long a timeout lasts.
const remainingLabel = (millis: number): string => {
    const minutes = Math.ceil(millis / 60000);
    if (minutes < 60) {
        return plural(minutes, 'minute');
    }
    if (minutes < 48 * 60) {
        return plural(Math.ceil(minutes / 60), 'hour');
    }
    return plural(Math.ceil(minutes / (24 * 60)), 'day');
};

// getPostingBlockedReason explains why the current user can't post in a
// channel, matching the server's enforcement, or returns an empty string.
export const getPostingBlockedReason = (state: GlobalState, channelId: string): string => {
    const status = getStatusForChannel(state, channelId);
    if (status.muted) {
        return 'You\'re muted in this channel. A moderator has to lift the mute before you can post again.';
    }
    const remaining = (status.timeout_until || 0) - Date.now();
    if (remaining > 0) {
        return `You're timed out in this channel. You can post again in ${remainingLabel(remaining)}.`;
    }
    return '';
};

export const getUserDisplayName = (state: GlobalState, userId: string): string => {
    const profile = state.entities.users.profiles[userId];
    if (!profile) {
        return 'this member';
    }
    if (profile.nickname) {
        return profile.nickname;
    }
    const fullName = [profile.first_name, profile.last_name].filter(Boolean).join(' ');
    return fullName || `@${profile.username}`;
};
