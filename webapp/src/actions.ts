import type {GlobalState} from '@mattermost/types/store';

import ActionTypes from './action_types';
import * as client from './client';
import {getStatusForChannel, pluginState} from './selectors';
import type {ConfirmAction, ConfirmModalState, ReportModalState, TimeoutModalState} from './types';

type GetState = () => GlobalState;

// The webapp store dispatch supports thunks; `unknown` keeps plain actions
// and nested thunks interchangeable without fighting redux's typings.
type ThunkDispatch = (action: unknown) => Promise<unknown> | unknown;

let toastTimer: ReturnType<typeof setTimeout>;

export const showToast = (message: string, icon = 'check-circle') => {
    return (dispatch: ThunkDispatch) => {
        clearTimeout(toastTimer);
        dispatch({type: ActionTypes.SHOW_TOAST, data: {message, icon}});
        toastTimer = setTimeout(() => dispatch({type: ActionTypes.HIDE_TOAST}), 2800);
    };
};

export const showErrorToast = (err: unknown) => {
    return showToast(client.errorMessage(err), 'alert-circle-outline');
};

export const fetchStatus = (channelId: string) => {
    return async (dispatch: ThunkDispatch) => {
        if (!channelId) {
            return;
        }
        try {
            const data = await client.fetchStatus(channelId);
            dispatch({type: ActionTypes.RECEIVED_STATUS, channelId, data});
        } catch {
            // Not a member of the channel or the request failed; leave the
            // default (no tools) status in place.
        }
    };
};

export const fetchReports = (channelId: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            const data = await client.fetchReports(channelId);
            dispatch({type: ActionTypes.RECEIVED_REPORTS, channelId, data});
        } catch {
            // The user may have lost moderator rights; ignore.
        }
    };
};

export const fetchMembers = (channelId: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            const data = await client.fetchMembers(channelId);
            dispatch({type: ActionTypes.RECEIVED_MEMBERS, channelId, data});
        } catch {
            // Admin-only endpoint; ignore.
        }
    };
};

export const setRHSTab = (tab: 'reports' | 'members') => ({type: ActionTypes.SET_RHS_TAB, data: tab});

export const openReportModal = (data: ReportModalState) => ({type: ActionTypes.OPEN_REPORT_MODAL, data});
export const closeReportModal = () => ({type: ActionTypes.CLOSE_REPORT_MODAL});
export const openTimeoutModal = (data: TimeoutModalState) => ({type: ActionTypes.OPEN_TIMEOUT_MODAL, data});
export const closeTimeoutModal = () => ({type: ActionTypes.CLOSE_TIMEOUT_MODAL});
export const openConfirmModal = (data: ConfirmModalState) => ({type: ActionTypes.OPEN_CONFIRM_MODAL, data});
export const closeConfirmModal = () => ({type: ActionTypes.CLOSE_CONFIRM_MODAL});

// refreshModerationData refetches whatever moderation data is already loaded
// for a channel, so the queue and the members tab stay current after actions
// and websocket events.
export const refreshModerationData = (channelId: string) => {
    return async (dispatch: ThunkDispatch, getState: GetState) => {
        const status = getStatusForChannel(getState(), channelId);
        await dispatch(fetchStatus(channelId));
        if (status.is_moderator) {
            await dispatch(fetchReports(channelId));
        }
        if (status.is_admin && pluginState(getState())?.members[channelId]) {
            await dispatch(fetchMembers(channelId));
        }
    };
};

export const submitReport = (modal: ReportModalState, reason: string, note: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.createReport({
                channel_id: modal.channelId,
                post_id: modal.postId,
                target_user_id: modal.targetUserId,
                reason,
                note,
            });
            dispatch(closeReportModal());
            dispatch(showToast('Report submitted — moderators have been notified'));
            dispatch(refreshModerationData(modal.channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const muteUser = (channelId: string, userId: string, targetName: string, reportId?: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.memberAction('mute', {channel_id: channelId, user_id: userId, report_id: reportId});
            dispatch(showToast(`Muted ${targetName} in this channel`, 'volume-off'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const submitTimeout = (modal: TimeoutModalState, duration: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            const result = await client.memberAction('timeout', {
                channel_id: modal.channelId,
                user_id: modal.targetUserId,
                duration,
                report_id: modal.reportId,
            }) as {label?: string};
            dispatch(closeTimeoutModal());
            dispatch(showToast(`${modal.targetName} timed out for ${result.label || duration}`, 'clock'));
            dispatch(refreshModerationData(modal.channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const liftRestrictions = (channelId: string, userId: string, targetName: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.memberAction('lift', {channel_id: channelId, user_id: userId});
            dispatch(showToast(`Restrictions lifted for ${targetName}`, 'check-circle'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const warnUser = (channelId: string, userId: string, targetName: string, reportId?: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.memberAction('warn', {channel_id: channelId, user_id: userId, report_id: reportId});
            dispatch(showToast(`Warning sent to ${targetName}`, 'account-alert-outline'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const escalateUser = (channelId: string, userId: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.memberAction('escalate', {channel_id: channelId, user_id: userId});
            dispatch(showToast('Escalated to admins for review', 'arrow-up'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const dismissReport = (channelId: string, reportId: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.dismissReport(channelId, reportId);
            dispatch(showToast('Report dismissed'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const setModerator = (channelId: string, userId: string, targetName: string, moderator: boolean) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.setModerator(channelId, userId, moderator);
            dispatch(showToast(moderator ? `${targetName} is now a moderator` : `${targetName} is no longer a moderator`, moderator ? 'shield' : 'account-outline'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const unbanUser = (channelId: string, userId: string, targetName: string) => {
    return async (dispatch: ThunkDispatch) => {
        try {
            await client.memberAction('unban', {channel_id: channelId, user_id: userId});
            dispatch(showToast(`${targetName} has been unbanned`, 'shield'));
            dispatch(refreshModerationData(channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

// runConfirmAction executes the action approved in the confirm modal.
export const runConfirmAction = (action: ConfirmAction) => {
    return async (dispatch: ThunkDispatch) => {
        dispatch(closeConfirmModal());
        try {
            switch (action.kind) {
            case 'delete_post':
                await client.memberAction('delete_post', {post_id: action.postId, channel_id: action.channelId, report_id: action.reportId});
                dispatch(showToast('Message deleted', 'trash-can-outline'));
                break;
            case 'ban':
                await client.memberAction('ban', {channel_id: action.channelId, user_id: action.targetUserId, report_id: action.reportId});
                dispatch(showToast(`${action.targetName} has been banned from the server`, 'cancel'));
                break;
            case 'remove':
                await client.memberAction('remove', {channel_id: action.channelId, user_id: action.targetUserId});
                dispatch(showToast(`${action.targetName} removed from this channel`, 'close'));
                break;
            }
            dispatch(refreshModerationData(action.channelId));
        } catch (err) {
            dispatch(showErrorToast(err));
        }
    };
};

export const confirmDeleteMessage = (channelId: string, postId: string, reportId?: string) => {
    return openConfirmModal({
        icon: 'trash-can-outline',
        title: 'Delete this message?',
        body: "This removes the message for everyone in the channel and notifies the author. This can't be undone.",
        confirmLabel: 'Delete message',
        action: {kind: 'delete_post', channelId, postId, reportId},
    });
};

export const confirmBan = (channelId: string, targetUserId: string, targetName: string, reportId?: string) => {
    return openConfirmModal({
        icon: 'cancel',
        title: `Ban ${targetName}?`,
        body: `${targetName} will be removed from every channel and blocked from signing back in. You can reverse this later from the member list.`,
        confirmLabel: 'Ban member',
        action: {kind: 'ban', channelId, targetUserId, targetName, reportId},
    });
};

export const confirmRemove = (channelId: string, targetUserId: string, targetName: string) => {
    return openConfirmModal({
        icon: 'close',
        title: `Remove ${targetName} from this channel?`,
        body: `${targetName} loses access to this channel but keeps their account and other channels.`,
        confirmLabel: 'Remove from channel',
        action: {kind: 'remove', channelId, targetUserId, targetName},
    });
};
