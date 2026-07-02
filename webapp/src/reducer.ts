import type {UnknownAction} from 'redux';

import ActionTypes from './action_types';
import type {ModerationState} from './types';

const initialState: ModerationState = {
    statuses: {},
    reports: {},
    members: {},
    rhsTab: 'reports',
    reportModal: null,
    timeoutModal: null,
    confirmModal: null,
    toast: null,
};

type ModerationAction = UnknownAction & {
    channelId?: string;
    data?: unknown;
};

export default function reducer(state = initialState, action: ModerationAction): ModerationState {
    switch (action.type) {
    case ActionTypes.RECEIVED_STATUS:
        return {
            ...state,
            statuses: {...state.statuses, [action.channelId as string]: action.data as ModerationState['statuses'][string]},
        };
    case ActionTypes.RECEIVED_REPORTS:
        return {
            ...state,
            reports: {...state.reports, [action.channelId as string]: action.data as ModerationState['reports'][string]},
        };
    case ActionTypes.RECEIVED_MEMBERS:
        return {
            ...state,
            members: {...state.members, [action.channelId as string]: action.data as ModerationState['members'][string]},
        };
    case ActionTypes.SET_RHS_TAB:
        return {...state, rhsTab: action.data as ModerationState['rhsTab']};
    case ActionTypes.OPEN_REPORT_MODAL:
        return {...state, reportModal: action.data as ModerationState['reportModal']};
    case ActionTypes.CLOSE_REPORT_MODAL:
        return {...state, reportModal: null};
    case ActionTypes.OPEN_TIMEOUT_MODAL:
        return {...state, timeoutModal: action.data as ModerationState['timeoutModal']};
    case ActionTypes.CLOSE_TIMEOUT_MODAL:
        return {...state, timeoutModal: null};
    case ActionTypes.OPEN_CONFIRM_MODAL:
        return {...state, confirmModal: action.data as ModerationState['confirmModal']};
    case ActionTypes.CLOSE_CONFIRM_MODAL:
        return {...state, confirmModal: null};
    case ActionTypes.SHOW_TOAST:
        return {...state, toast: action.data as ModerationState['toast']};
    case ActionTypes.HIDE_TOAST:
        return {...state, toast: null};
    default:
        return state;
    }
}
