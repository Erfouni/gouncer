// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
	LastPrivilegedError,
	RoleBeyondReachError,
	RoleChangedError,
	RoleRefusedError,
	RoleUnknownError,
	ValidationError,
	createUser,
	invite,
	setUserDisabled,
	setUserRole,
} from '../src/admin'
import { HttpResponse, http, server } from '../src/testing'

const id = '0198b2f0-0000-7000-8000-000000000002'

const writes = [
	{
		name: 'createUser',
		failure: 'creating user',
		handler: (response: () => Response) => http.post('/api/users', response),
		run: () =>
			createUser({ email: 'maria@example.com', name: 'Maria Perez', password: 'correct horse battery', role: 'x' }),
	},
	{
		name: 'invite',
		failure: 'inviting',
		handler: (response: () => Response) => http.post('/api/users/invite', response),
		run: () => invite({ email: 'maria@example.com', name: 'Maria Perez', role: 'x' }),
	},
	{
		name: 'setUserRole',
		failure: 'updating role',
		handler: (response: () => Response) => http.put('/api/users/:id/role', response),
		run: () => setUserRole(id, 'x'),
	},
	{
		name: 'setUserDisabled',
		failure: 'updating user',
		handler: (response: () => Response) => http.patch('/api/users/:id', response),
		run: () => setUserDisabled(id, false),
	},
]

describe.each(writes)('$name', ({ failure, handler, run }) => {
	it('throws RoleUnknownError for a role nobody declared', async () => {
		server.use(
			handler(() =>
				HttpResponse.json(
					{ error: 'unknown role', code: 'role_unknown', meta: { field: 'role' } },
					{ status: 422 },
				),
			),
		)

		const attempt = run()

		await expect(attempt).rejects.toBeInstanceOf(RoleUnknownError)
		await expect(attempt).rejects.toThrow('unknown role')
	})

	it('throws RoleBeyondReachError for a role beyond the actor', async () => {
		server.use(
			handler(() =>
				HttpResponse.json({ error: 'role beyond reach', code: 'role_beyond_reach' }, { status: 403 }),
			),
		)

		const attempt = run()

		await expect(attempt).rejects.toBeInstanceOf(RoleBeyondReachError)
		await expect(attempt).rejects.toThrow('role beyond reach')
	})

	it.each(['constructor', 'toString', '__proto__', 'hasOwnProperty'])(
		'answers the code %s as a plain error naming the status',
		async (code) => {
			server.use(handler(() => HttpResponse.json({ error: 'odd', code }, { status: 403 })))

			await expect(run()).rejects.toThrow(`${failure} failed with status 403`)
		},
	)
})

describe.each(writes.filter(({ name }) => name === 'setUserRole' || name === 'setUserDisabled'))(
	'$name on an account whose role moved',
	({ handler, run }) => {
		it('throws RoleChangedError', async () => {
			server.use(
				handler(() =>
					HttpResponse.json(
						{ error: 'the role changed, reload the account', code: 'role_changed' },
						{ status: 409 },
					),
				),
			)

			const attempt = run()

			await expect(attempt).rejects.toBeInstanceOf(RoleChangedError)
			await expect(attempt).rejects.toThrow('the role changed, reload the account')
		})
	},
)

describe('RoleUnknownError', () => {
	it('stays a ValidationError, so a screen that reads only that still shows the message', async () => {
		server.use(
			http.post('/api/users', () =>
				HttpResponse.json({ error: 'unknown role', code: 'role_unknown' }, { status: 422 }),
			),
		)

		await expect(
			createUser({ email: 'maria@example.com', name: 'Maria Perez', password: 'correct horse battery' }),
		).rejects.toBeInstanceOf(ValidationError)
	})
})

describe('an error carrying another code', () => {
	it('keeps createUser answering any other 403 as a plain error', async () => {
		server.use(
			http.post('/api/users', () =>
				HttpResponse.json({ error: 'role insufficient', code: 'role_insufficient' }, { status: 403 }),
			),
		)

		const attempt = createUser({ email: 'maria@example.com', name: 'Maria Perez', password: 'correct horse battery' })

		await expect(attempt).rejects.toThrow('403')
		await expect(attempt).rejects.not.toBeInstanceOf(RoleBeyondReachError)
	})

	it('keeps invite answering a role_insufficient 403 as a plain error', async () => {
		server.use(
			http.post('/api/users/invite', () =>
				HttpResponse.json({ error: 'role insufficient', code: 'role_insufficient' }, { status: 403 }),
			),
		)

		const attempt = invite({ email: 'maria@example.com', name: 'Maria Perez' })

		await expect(attempt).rejects.toThrow('inviting failed with status 403')
		await expect(attempt).rejects.not.toBeInstanceOf(RoleRefusedError)
	})

	it('keeps setUserDisabled answering a role_insufficient 403 as a plain error', async () => {
		server.use(
			http.patch('/api/users/:id', () =>
				HttpResponse.json({ error: 'role insufficient', code: 'role_insufficient' }, { status: 403 }),
			),
		)

		const attempt = setUserDisabled(id, true)

		await expect(attempt).rejects.toThrow('updating user failed with status 403')
		await expect(attempt).rejects.not.toBeInstanceOf(RoleRefusedError)
	})

	it('keeps setUserDisabled answering a last_privileged_refused 422 as a plain error', async () => {
		server.use(
			http.patch('/api/users/:id', () =>
				HttpResponse.json(
					{ error: 'the last privileged account must remain', code: 'last_privileged_refused' },
					{ status: 422 },
				),
			),
		)

		const attempt = setUserDisabled(id, true)

		await expect(attempt).rejects.toThrow('updating user failed with status 422')
		await expect(attempt).rejects.not.toBeInstanceOf(ValidationError)
		await expect(attempt).rejects.not.toBeInstanceOf(LastPrivilegedError)
	})

	it('keeps setUserDisabled answering an unreadable error body by its status', async () => {
		server.use(http.patch('/api/users/:id', () => new HttpResponse('not json at all', { status: 403 })))

		await expect(setUserDisabled(id, true)).rejects.toThrow('403')
	})
})
