import React from 'react';
import {useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import ConfirmModal from './confirm_modal';
import ReportModal from './report_modal';
import TimeoutModal from './timeout_modal';

import {pluginState} from '../../selectors';
import Icon from '../icons';
import {C} from '../styles';

// ModerationRoot renders the plugin's modals and the toast. It is registered
// as a root component so it lives above the channel view.
const ModerationRoot = () => {
    const state = useSelector((s: GlobalState) => pluginState(s));

    if (!state) {
        return null;
    }

    return (
        <>
            {state.reportModal && <ReportModal modal={state.reportModal}/>}
            {state.timeoutModal && <TimeoutModal modal={state.timeoutModal}/>}
            {state.confirmModal && <ConfirmModal modal={state.confirmModal}/>}
            {state.toast && (
                <div style={{position: 'fixed', bottom: 26, left: '50%', transform: 'translateX(-50%)', zIndex: 1200, display: 'flex', alignItems: 'center', gap: 10, background: '#2d3039', color: '#fff', padding: '12px 18px', borderRadius: 8, boxShadow: '0 12px 32px rgba(0,0,0,0.24)', maxWidth: 420}}>
                    <Icon
                        name={state.toast.icon}
                        size={18}
                        color={state.toast.icon === 'alert-circle-outline' ? '#f5786c' : C.green500}
                    />
                    <span style={{fontSize: 14, fontWeight: 500}}>{state.toast.message}</span>
                </div>
            )}
        </>
    );
};

export default ModerationRoot;
