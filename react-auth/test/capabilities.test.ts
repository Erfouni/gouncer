// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import { can } from '../src'
import { fetchSession } from '../src/api'
import type { User } from '../src/api'
import { HttpResponse, http, server } from '../src/testing'

const maria = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'maria.perez@example.com',
	name: 'Maria Perez',
	role: 'editor',
}

describe('the capabilities on a session', () => {
	it('carries the capabilities and grantable roles the server sends, in order', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({
					...maria,
					capabilities: ['posts.edit', 'posts.publish'],
					grantable: ['editor', 'author', 'member'],
				}),
			),
		)

		const held = await fetchSession()

		expect(held?.capabilities).toEqual(['posts.edit', 'posts.publish'])
		expect(held?.grantable).toEqual(['editor', 'author', 'member'])
	})

	it('reads a server that sends neither key as carrying none', async () => {
		server.use(http.get('/api/auth/session', () => HttpResponse.json(maria)))

		const held = await fetchSession()

		expect(held).toEqual(maria)
		expect(can(held, 'posts.edit')).toBe(false)
	})

	it('keeps empty lists empty', async () => {
		server.use(
			http.get('/api/auth/session', () => HttpResponse.json({ ...maria, capabilities: [], grantable: [] })),
		)

		const held = await fetchSession()

		expect(held?.capabilities).toEqual([])
		expect(held?.grantable).toEqual([])
	})
})

describe('can', () => {
	it('answers true for a capability the user holds', () => {
		expect(can({ ...maria, capabilities: ['posts.edit'] }, 'posts.edit')).toBe(true)
	})

	it('answers false for a capability the user lacks', () => {
		expect(can({ ...maria, capabilities: ['posts.edit'] }, 'users.manage')).toBe(false)
	})

	it('answers false for a user carrying no capabilities', () => {
		const plain: User = maria

		expect(can(plain, 'posts.edit')).toBe(false)
	})

	it('answers false without a user', () => {
		expect(can(null, 'posts.edit')).toBe(false)
		expect(can(undefined, 'posts.edit')).toBe(false)
	})
})
