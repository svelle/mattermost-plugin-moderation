import React from 'react';
import {useDispatch} from 'react-redux';

import {closeConfirmModal, runConfirmAction} from '../../actions';
import type {ConfirmModalState} from '../../types';
import Icon from '../icons';
import {C} from '../styles';

type Props = {
    modal: ConfirmModalState;
};

const ConfirmModal = ({modal}: Props) => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => void;
    const close = () => dispatch(closeConfirmModal());

    return (
        <div
            onClick={close}
            style={{position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.48)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1100}}
        >
            <div
                onClick={(e) => e.stopPropagation()}
                role='dialog'
                aria-label={modal.title}
                style={{width: 432, maxWidth: 'calc(100vw - 32px)', background: C.centerBg, color: C.fg1, borderRadius: 8, boxShadow: '0 12px 32px rgba(0,0,0,0.24)', padding: '22px 24px'}}
            >
                <div style={{display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 14}}>
                    <span style={{width: 40, height: 40, borderRadius: 8, background: C.redTint, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
                        <Icon
                            name={modal.icon}
                            size={22}
                            color={C.red500}
                        />
                    </span>
                    <div style={{flex: 1}}>
                        <div style={{fontWeight: 600, fontSize: 19, lineHeight: 1.2}}>{modal.title}</div>
                    </div>
                </div>
                <div style={{fontSize: 14, lineHeight: '20px', color: C.fg2, marginBottom: 20}}>{modal.body}</div>
                <div style={{display: 'flex', justifyContent: 'flex-end', gap: 8}}>
                    <button
                        onClick={close}
                        style={{height: 40, padding: '0 18px', borderRadius: 4, background: 'transparent', color: C.fg1, boxShadow: `inset 0 0 0 1px ${C.border3}`, border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: 14, fontFamily: 'inherit'}}
                    >
                        {'Cancel'}
                    </button>
                    <button
                        onClick={() => dispatchThunk(runConfirmAction(modal.action))}
                        style={{height: 40, padding: '0 18px', borderRadius: 4, background: C.red500, color: '#fff', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: 14, fontFamily: 'inherit'}}
                    >
                        {modal.confirmLabel}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default ConfirmModal;
