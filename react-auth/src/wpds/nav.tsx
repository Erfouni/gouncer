// SPDX-License-Identifier: Apache-2.0

import { _x } from '@wordpress/i18n'
import { people } from '@wordpress/icons'
import type { ComponentProps, ReactElement } from 'react'

import { DOMAIN } from '../domain.js'

/** The ready-made menu row for the user administration screen. */
export const usersMenuItem: {
	label: string
	to: string
	icon: ReactElement<ComponentProps<'svg'>>
} = {
	get label() {
		return _x('Users', 'admin section', DOMAIN)
	},
	to: '/users',
	icon: people,
}
