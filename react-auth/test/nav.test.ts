// SPDX-License-Identifier: Apache-2.0

import { people } from '@wordpress/icons'
import { expect, test } from 'vitest'

import * as wpds from '../src/wpds'

test('draws the Users row with the people glyph WordPress draws', () => {
	expect(wpds.usersMenuItem.icon).toBe(people)
})

test('keeps no copy of the row under its old name', () => {
	expect(wpds).not.toHaveProperty('usersNavItem')
})
