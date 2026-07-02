// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';
import React from 'react';
import type {Store} from 'redux';

import type {Post} from '@mattermost/types/posts';
import type {ProductScope} from '@mattermost/types/products';
import type {GlobalState} from '@mattermost/types/store';
import type {UserProfile} from '@mattermost/types/users';

import type {PluginRegistry} from 'types/mattermost-webapp';

import {
    confirmBan,
    confirmDeleteMessage,
    escalateUser,
    fetchStatus,
    muteUser,
    openReportModal,
    openTimeoutModal,
    refreshModerationData,
    warnUser,
} from './actions';
import {setBasePath} from './client';
import ChannelHeaderButton from './components/channel_header_button';
import {SHIELD_PERSON} from './components/icons';
import ModerationRoot from './components/modals/root';
import RHSPanel from './components/rhs';
import PopoverUserActions from './components/user_actions';
import reducer from './reducer';
import {getCurrentChannelId, getStatusForChannel} from './selectors';

const MAX_EXCERPT_LENGTH = 300;

const displayNameOf = (profile?: UserProfile): string => {
    if (!profile) {
        return 'this member';
    }
    if (profile.nickname) {
        return profile.nickname;
    }
    const fullName = [profile.first_name, profile.last_name].filter(Boolean).join(' ');
    return fullName || `@${profile.username}`;
};

export default class Plugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        setBasePath(store.getState().entities.general.config.SiteURL);

        registry.registerReducer(reducer);

        const {toggleRHSPlugin} = registry.registerRightHandSidebarComponent(
            RHSPanel,
            'Moderation',
        );

        registry.registerChannelHeaderButtonAction(
            <ChannelHeaderButton/>,
            () => store.dispatch(toggleRHSPlugin),
            'Moderation',
            'Community moderation',
        );

        // The app bar renders plugin icons on a fixed light circle, so it
        // needs a self-contained image with an explicit fill rather than the
        // theme-following channel header icon. Registering it also stops the
        // app bar from reusing the channel header button as a fallback.
        if (typeof registry.registerAppBarComponent === 'function') {
            const appBarIconUrl = `data:image/svg+xml,${encodeURIComponent(
                `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${SHIELD_PERSON.viewBox}" fill="#1c58d9"><path d="${SHIELD_PERSON.d}"/></svg>`,
            )}`;
            registry.registerAppBarComponent(
                appBarIconUrl,
                () => store.dispatch(toggleRHSPlugin),
                'Community moderation',
                null as unknown as ProductScope,
            );
        }

        registry.registerRootComponent(ModerationRoot);
        registry.registerPopoverUserActionsComponent(PopoverUserActions);

        const dispatch = store.dispatch as (action: unknown) => void;

        // The webapp passes a post ID to dropdown menu callbacks in current
        // releases and a post object in some older ones — handle both.
        const getPost = (postOrId: Post | string): Post | undefined => {
            if (typeof postOrId !== 'string') {
                return postOrId;
            }
            return store.getState().entities.posts.posts[postOrId];
        };

        const authorName = (post: Post): string => {
            return displayNameOf(store.getState().entities.users.profiles[post.user_id]);
        };

        // isModeratablePost is the base filter for every menu item: a real,
        // non-system message written by someone else.
        const isModeratablePost = (postOrId: Post | string): Post | null => {
            const post = getPost(postOrId);
            if (!post || !post.user_id || post.type || post.delete_at) {
                return null;
            }
            if (post.user_id === store.getState().entities.users.currentUserId) {
                return null;
            }
            return post;
        };

        const channelStatus = (post: Post) => getStatusForChannel(store.getState(), post.channel_id);

        const registerPostAction = (
            text: string,
            handler: (post: Post) => void,
            check: (post: Post) => boolean,
        ) => {
            registry.registerPostDropdownMenuAction(
                text,
                ((postOrId: Post | string) => {
                    const post = isModeratablePost(postOrId);
                    if (post) {
                        handler(post);
                    }
                }) as () => void,
                ((postOrId: Post | string) => {
                    const post = isModeratablePost(postOrId);
                    return Boolean(post && check(post));
                }) as (post: Post) => boolean,
            );
        };

        registerPostAction(
            'Report message',
            (post) => dispatch(openReportModal({
                channelId: post.channel_id,
                postId: post.id,
                targetUserId: post.user_id,
                targetName: authorName(post),
                excerpt: post.message.length > MAX_EXCERPT_LENGTH ? `${post.message.slice(0, MAX_EXCERPT_LENGTH - 1)}…` : post.message,
            })),
            () => true,
        );

        registerPostAction(
            'Report user',
            (post) => dispatch(openReportModal({
                channelId: post.channel_id,
                targetUserId: post.user_id,
                targetName: authorName(post),
            })),
            () => true,
        );

        registerPostAction(
            'Mute in channel',
            (post) => dispatch(muteUser(post.channel_id, post.user_id, authorName(post))),
            (post) => channelStatus(post).is_moderator,
        );

        registerPostAction(
            'Timeout member…',
            (post) => dispatch(openTimeoutModal({
                channelId: post.channel_id,
                targetUserId: post.user_id,
                targetName: authorName(post),
            })),
            (post) => channelStatus(post).is_moderator,
        );

        registerPostAction(
            'Warn member',
            (post) => dispatch(warnUser(post.channel_id, post.user_id, authorName(post))),
            (post) => channelStatus(post).is_moderator,
        );

        registerPostAction(
            'Delete message (moderation)',
            (post) => dispatch(confirmDeleteMessage(post.channel_id, post.id)),
            (post) => channelStatus(post).is_moderator,
        );

        registerPostAction(
            'Escalate to admin',
            (post) => dispatch(escalateUser(post.channel_id, post.user_id)),
            (post) => {
                const status = channelStatus(post);
                return status.is_moderator && !status.is_admin;
            },
        );

        registerPostAction(
            'Ban from server',
            (post) => dispatch(confirmBan(post.channel_id, post.user_id, authorName(post))),
            (post) => channelStatus(post).is_admin,
        );

        registry.registerWebSocketEventHandler(
            `custom_${manifest.id}_reports_changed`,
            (msg) => {
                const channelId = msg.data?.channel_id;
                if (channelId) {
                    dispatch(refreshModerationData(channelId));
                }
            },
        );

        registry.registerWebSocketEventHandler(
            `custom_${manifest.id}_members_changed`,
            (msg) => {
                const channelId = msg.data?.channel_id;
                if (channelId) {
                    dispatch(refreshModerationData(channelId));
                }
            },
        );

        registry.registerReconnectHandler(() => {
            dispatch(fetchStatus(getCurrentChannelId(store.getState())));
        });
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
