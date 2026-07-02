import React, {useEffect, useState} from 'react';
import {useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import MemberRow from './member_row';

import * as client from '../../client';
import {pluginState} from '../../selectors';
import type {MemberInfo} from '../../types';
import Icon from '../icons';
import {C} from '../styles';

type Props = {
    channelId: string;
    currentUserId: string;
};

// MembersTab lists members with an elevated role or an active restriction,
// and lets admins search all channel members to elevate or manage anyone.
const MembersTab = ({channelId, currentUserId}: Props) => {
    const members = useSelector((state: GlobalState) => pluginState(state)?.members[channelId]);
    const [term, setTerm] = useState('');
    const [results, setResults] = useState<MemberInfo[] | null>(null);
    const [searching, setSearching] = useState(false);

    const trimmed = term.trim();

    useEffect(() => {
        if (!trimmed) {
            setResults(null);
            setSearching(false);
            return undefined;
        }
        setSearching(true);
        let cancelled = false;
        const timer = setTimeout(async () => {
            try {
                const found = await client.fetchMembers(channelId, trimmed);
                if (!cancelled) {
                    setResults(found);
                }
            } catch {
                if (!cancelled) {
                    setResults([]);
                }
            } finally {
                if (!cancelled) {
                    setSearching(false);
                }
            }
        }, 250);
        return () => {
            cancelled = true;
            clearTimeout(timer);
        };

        // `members` is a dependency so search results refresh after an action
        // (e.g. making a moderator) triggers a members refetch.
    }, [trimmed, channelId, members]);

    const showingSearch = trimmed.length > 0;
    const rows = showingSearch ? (results || []) : (members || []);

    return (
        <div style={{flex: 1, overflowY: 'auto', padding: '12px 12px'}}>
            <div style={{fontSize: 12, color: C.fg3, lineHeight: 1.4, padding: '2px 4px 10px'}}>
                {'Assign moderators and manage members without opening the System Console.'}
            </div>
            <div style={{display: 'flex', alignItems: 'center', gap: 8, height: 34, padding: '0 10px', border: `1px solid ${C.border2}`, borderRadius: 5, background: C.centerBg, marginBottom: 10}}>
                <i
                    className='icon icon-magnify'
                    style={{fontSize: 15, color: C.fg3, lineHeight: 1}}
                />
                <input
                    value={term}
                    onChange={(e) => setTerm(e.target.value)}
                    placeholder='Search members to change their role…'
                    style={{flex: 1, border: 'none', outline: 'none', background: 'transparent', fontFamily: 'inherit', fontSize: 13, color: C.fg1}}
                />
                {showingSearch && (
                    <button
                        onClick={() => setTerm('')}
                        aria-label='Clear search'
                        style={{border: 'none', background: 'transparent', cursor: 'pointer', padding: 0, display: 'flex', color: C.fg3}}
                    >
                        <Icon
                            name='close'
                            size={14}
                        />
                    </button>
                )}
            </div>

            {!showingSearch && (
                <div style={{fontSize: 11, color: C.fg3, lineHeight: 1.4, padding: '0 4px 10px'}}>
                    {'Showing moderators, admins, and members with active restrictions. Search to find anyone else.'}
                </div>
            )}

            {showingSearch && searching && (
                <div style={{padding: '18px 4px', fontSize: 13, color: C.fg3}}>{'Searching…'}</div>
            )}

            {showingSearch && !searching && rows.length === 0 && (
                <div style={{padding: '18px 4px', fontSize: 13, color: C.fg3}}>
                    {'No members of this channel match '}<b>{trimmed}</b>{'.'}
                </div>
            )}

            {!showingSearch && rows.length === 0 && (
                <div style={{padding: '18px 4px', fontSize: 13, color: C.fg3, lineHeight: 1.5}}>
                    {'Nobody here has an elevated role or an active restriction yet. Search above to find a member and make them a moderator.'}
                </div>
            )}

            {rows.map((member) => (
                <MemberRow
                    key={member.user_id}
                    member={member}
                    channelId={channelId}
                    currentUserId={currentUserId}
                />
            ))}
        </div>
    );
};

export default MembersTab;
