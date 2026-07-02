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

export const pluginState = (state: GlobalState): ModerationState => {
    return (state as StateWithPlugin)[`plugins-${manifest.id}`] as ModerationState;
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
