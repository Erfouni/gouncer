// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
	RateLimitedError,
	SetupClosedError,
	SetupCodeError,
	SiteStartingError,
	ValidationError,
	checkSetupCode,
	createFirstAccount,
	isSetupOpen,
} from '../src/index'
import {
	HttpResponse,
	createdAccount,
	firstAccountClosed,
	firstAccountCodeRefused,
	firstAccountOk,
	firstAccountRateLimited,
	firstAccountStarting,
	http,
	server,
	setupCodeClosed,
	setupCodeOk,
	setupCodeRateLimited,
	setupCodeRefused,
	setupCodeStarting,
	setupStatus,
} from '../src/testing'

const firstAccount = {
	code: 'K7QW3MZTB2XQ5LJ4N6RAHYCDEF',
	email: 'maria@example.com',
	name: 'Maria Perez',
	password: 'correct horse battery',
}

const createdBody = {
	id: '0198b2f0-0000-7000-8000-000000000002',
	email: 'maria@example.com',
	name: 'Maria Perez',
	disabled: false,
	confirmed: true,
	role: 'owner',
	created_at: '2026-10-19T10:00:00Z',
}

const codeRefusals = [
	{ status: 401, code: 'setup_code_invalid', error: 'setup code invalid', reason: 'invalid' },
	{ status: 401, code: 'setup_code_expired', error: 'setup code expired', reason: 'expired' },
	{ status: 500, code: 'setup_code_file', error: 'setup code file unusable', reason: 'file' },
] as const

const closedBody = { error: 'setup already done', code: 'setup_done' }

const throttledBody = { error: 'too many login attempts, try again later', code: 'login_rate_limited' }

function answer(path: string, status: number, body: object | null, headers: Record<string, string> = {}) {
	return http.post(path, () =>
		body === null ? new HttpResponse(null, { status, headers }) : HttpResponse.json(body, { status, headers }),
	)
}

async function refusedAs<T>(attempt: Promise<unknown>, type: abstract new (...args: never[]) => T): Promise<T> {
	const refused = await attempt.then(
		() => undefined,
		(held: unknown) => held,
	)
	expect(refused).toBeInstanceOf(type)
	return refused as T
}

describe('isSetupOpen', () => {
	it.each([true, false])('reads the open flag as %s', async (open) => {
		server.use(http.get('/api/auth/setup', () => HttpResponse.json({ open })))

		await expect(isSetupOpen()).resolves.toBe(open)
	})

	it('throws when the site fails to answer the flag', async () => {
		server.use(
			http.get('/api/auth/setup', () => HttpResponse.json({ error: 'internal error', code: 'internal' }, { status: 500 })),
		)

		await expect(isSetupOpen()).rejects.toThrow('500')
	})

	it('hands its abort signal to the request', async () => {
		server.use(setupStatus(true))
		const controller = new AbortController()
		controller.abort()

		await expect(isSetupOpen(controller.signal)).rejects.toThrow()
	})
})

describe('createFirstAccount', () => {
	it('posts exactly the input and answers the account', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/setup', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json(createdBody, { status: 201 })
			}),
		)
		const carryingMore = { ...firstAccount, role: 'owner', confirmed: true }

		const account = await createFirstAccount(carryingMore)

		expect(body).toEqual(firstAccount)
		expect(account.email).toBe('maria@example.com')
	})

	it('reads the account the site answers', async () => {
		server.use(answer('/api/auth/setup', 201, createdBody))

		const account = await createFirstAccount(firstAccount)

		expect(account).toEqual({
			id: createdBody.id,
			email: 'maria@example.com',
			name: 'Maria Perez',
			disabled: false,
			role: 'owner',
			created_at: new Date('2026-10-19T10:00:00Z'),
		})
		expect(account.created_at).toBeInstanceOf(Date)
	})

	it('a wrong code throws SetupCodeError with the reason invalid', async () => {
		server.use(answer('/api/auth/setup', 401, { error: 'setup code invalid', code: 'setup_code_invalid' }))

		const refused = await refusedAs(createFirstAccount(firstAccount), SetupCodeError)

		expect(refused.reason).toBe('invalid')
	})

	it('a refused code with no readable reason throws SetupCodeError with the reason invalid', async () => {
		server.use(http.post('/api/auth/setup', () => new HttpResponse('refused', { status: 401 })))

		const refused = await refusedAs(createFirstAccount(firstAccount), SetupCodeError)

		expect(refused.reason).toBe('invalid')
	})

	it('an expired code throws SetupCodeError with the reason expired', async () => {
		server.use(answer('/api/auth/setup', 401, { error: 'setup code expired', code: 'setup_code_expired' }))

		const refused = await refusedAs(createFirstAccount(firstAccount), SetupCodeError)

		expect(refused.reason).toBe('expired')
	})

	it('a code file the site cannot use throws SetupCodeError with the reason file', async () => {
		server.use(answer('/api/auth/setup', 500, { error: 'setup code file unusable', code: 'setup_code_file' }))

		const refused = await refusedAs(createFirstAccount(firstAccount), SetupCodeError)

		expect(refused.reason).toBe('file')
	})

	it('a set-up site throws SetupClosedError', async () => {
		server.use(answer('/api/auth/setup', 409, closedBody))

		await expect(createFirstAccount(firstAccount)).rejects.toBeInstanceOf(SetupClosedError)
	})

	it('a throttled request throws RateLimitedError', async () => {
		server.use(answer('/api/auth/setup', 429, throttledBody, { 'Retry-After': '120' }))

		await expect(createFirstAccount(firstAccount)).rejects.toBeInstanceOf(RateLimitedError)
	})

	it('a short password throws ValidationError with its code and meta', async () => {
		server.use(
			answer('/api/auth/setup', 422, {
				error: 'password must be at least 12 characters',
				code: 'password_too_short',
				meta: { min: 12 },
			}),
		)

		const refused = await refusedAs(createFirstAccount({ ...firstAccount, password: 'short' }), ValidationError)

		expect(refused.message).toBe('password must be at least 12 characters')
		expect(refused.code).toBe('password_too_short')
		expect(refused.meta).toEqual({ min: 12 })
	})

	it('a starting site throws SiteStartingError with its wait', async () => {
		server.use(answer('/api/auth/setup', 503, null, { 'Retry-After': '5' }))
		const waiting = await refusedAs(createFirstAccount(firstAccount), SiteStartingError)
		server.use(answer('/api/auth/setup', 503, null))
		const unsaid = await refusedAs(createFirstAccount(firstAccount), SiteStartingError)
		server.use(answer('/api/auth/setup', 503, null, { 'Retry-After': 'Mon, 19 Oct 2026 10:00:00 GMT' }))
		const dated = await refusedAs(createFirstAccount(firstAccount), SiteStartingError)
		server.use(answer('/api/auth/setup', 503, null, { 'Retry-After': '0' }))
		const immediate = await refusedAs(createFirstAccount(firstAccount), SiteStartingError)

		expect(waiting.retryAfter).toBe(5)
		expect(unsaid.retryAfter).toBeUndefined()
		expect(dated.retryAfter).toBeUndefined()
		expect(immediate.retryAfter).toBe(0)
	})

	it('a server failure throws a plain error naming its status', async () => {
		server.use(answer('/api/auth/setup', 500, { error: 'internal error', code: 'internal' }))

		const refused = await refusedAs(createFirstAccount(firstAccount), Error)

		expect(refused).not.toBeInstanceOf(SetupCodeError)
		expect(refused.message).toContain('500')
	})
})

describe('checkSetupCode', () => {
	it('resolves when the site accepts the code', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/setup/code', async ({ request }) => {
				body = await request.json()
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await expect(checkSetupCode(' k7qw-3mzt b2xq5lj4n6rahycdef ')).resolves.toBeUndefined()
		expect(body).toEqual({ code: ' k7qw-3mzt b2xq5lj4n6rahycdef ' })
	})

	it.each(codeRefusals)('tells each refused code by its reason, $code as $reason', async (row) => {
		server.use(answer('/api/auth/setup/code', row.status, { error: row.error, code: row.code }))

		const refused = await refusedAs(checkSetupCode(firstAccount.code), SetupCodeError)

		expect(refused.reason).toBe(row.reason)
	})

	it('reports a set-up site, a throttled request and a starting site', async () => {
		server.use(answer('/api/auth/setup/code', 409, closedBody))
		await refusedAs(checkSetupCode(firstAccount.code), SetupClosedError)
		server.use(answer('/api/auth/setup/code', 429, throttledBody))
		await refusedAs(checkSetupCode(firstAccount.code), RateLimitedError)
		server.use(answer('/api/auth/setup/code', 503, null, { 'Retry-After': '5' }))
		const waiting = await refusedAs(checkSetupCode(firstAccount.code), SiteStartingError)
		server.use(answer('/api/auth/setup/code', 503, null))
		const unsaid = await refusedAs(checkSetupCode(firstAccount.code), SiteStartingError)

		expect(waiting.retryAfter).toBe(5)
		expect(unsaid.retryAfter).toBeUndefined()
	})
})

const reasons = ['invalid', 'expired', 'file'] as const

const cannedRows = [
	{
		name: 'setupStatus',
		run: async () => {
			server.use(setupStatus(true))
			await expect(isSetupOpen()).resolves.toBe(true)
			server.use(setupStatus(false))
			await expect(isSetupOpen()).resolves.toBe(false)
		},
	},
	{
		name: 'setupCodeOk',
		run: async () => {
			server.use(setupCodeOk())
			await expect(checkSetupCode(firstAccount.code)).resolves.toBeUndefined()
		},
	},
	{
		name: 'setupCodeRefused',
		run: async () => {
			for (const reason of reasons) {
				server.use(setupCodeRefused(reason))
				expect((await refusedAs(checkSetupCode(firstAccount.code), SetupCodeError)).reason).toBe(reason)
			}
		},
	},
	{
		name: 'setupCodeClosed',
		run: async () => {
			server.use(setupCodeClosed())
			await refusedAs(checkSetupCode(firstAccount.code), SetupClosedError)
		},
	},
	{
		name: 'setupCodeRateLimited',
		run: async () => {
			server.use(setupCodeRateLimited())
			await refusedAs(checkSetupCode(firstAccount.code), RateLimitedError)
		},
	},
	{
		name: 'setupCodeStarting',
		run: async () => {
			server.use(setupCodeStarting())
			expect((await refusedAs(checkSetupCode(firstAccount.code), SiteStartingError)).retryAfter).toBeUndefined()
		},
	},
	{
		name: 'firstAccountOk',
		run: async () => {
			server.use(firstAccountOk())
			await expect(createFirstAccount(firstAccount)).resolves.toEqual(createdAccount)
			const adminAccount = { ...createdAccount, role: 'admin' }
			server.use(firstAccountOk(adminAccount))
			await expect(createFirstAccount(firstAccount)).resolves.toEqual(adminAccount)
		},
	},
	{
		name: 'firstAccountCodeRefused',
		run: async () => {
			for (const reason of reasons) {
				server.use(firstAccountCodeRefused(reason))
				expect((await refusedAs(createFirstAccount(firstAccount), SetupCodeError)).reason).toBe(reason)
			}
		},
	},
	{
		name: 'firstAccountClosed',
		run: async () => {
			server.use(firstAccountClosed())
			await refusedAs(createFirstAccount(firstAccount), SetupClosedError)
		},
	},
	{
		name: 'firstAccountRateLimited',
		run: async () => {
			server.use(firstAccountRateLimited())
			await refusedAs(createFirstAccount(firstAccount), RateLimitedError)
		},
	},
	{
		name: 'firstAccountStarting',
		run: async () => {
			server.use(firstAccountStarting())
			expect((await refusedAs(createFirstAccount(firstAccount), SiteStartingError)).retryAfter).toBeUndefined()
		},
	},
]

describe('the canned setup handlers', () => {
	it.each(cannedRows)('answers through the canned setup handlers, $name', async ({ run }) => {
		await run()
	})

	it('cans the created account as Maria Perez', () => {
		expect(createdAccount).toMatchObject({ email: 'maria@example.com', name: 'Maria Perez', disabled: false })
	})
})
