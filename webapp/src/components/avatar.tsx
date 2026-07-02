import React from 'react';

import {getBasePath} from '../client';

type Props = {
    userId: string;
    size?: number;
};

const Avatar = ({userId, size = 34}: Props) => (
    <img
        src={`${getBasePath()}/api/v4/users/${userId}/image?_=0`}
        style={{width: size, height: size, borderRadius: '50%', flexShrink: 0, objectFit: 'cover'}}
        alt=''
    />
);

export default Avatar;
