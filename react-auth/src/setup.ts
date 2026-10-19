// SPDX-License-Identifier: Apache-2.0

import { z } from 'zod'

import { accountSchema } from './account.js'
import type { Account } from './account.js'
import { RateLimitedError, ValidationError, refusalOf } from './api.js'
import { resolveTransport } from './transport.js'

/** FirstAccount is what the setup account step sends. */
export interface FirstAccount {
	code: string
	email: string
	name: string
	password: string
}

/** SetupCodeError is thrown when the setup code is wrong, has expired, or its file cannot be used. */
export class SetupCodeError extends Error {
	readonly reason: 'invalid' | 'expired' | 'file'

	/**
	 * Creates the error for the reason the site gave.
	 * @param reason - Why the site refused the code.
	 */
	constructor(reason: 'invalid' | 'expired' | 'file') {
		super(`the setup code was refused as ${reason}`)
		this.reason = reason
	}
}

/** SetupClosedError is thrown when the site already has its first account. */
export class SetupClosedError extends Error {}

/** SiteStartingError is thrown when the site answers 503 while it starts. */
export class SiteStartingError extends Error {
	readonly retryAfter?: number

	/**
	 * Creates the error with the wait the site asked for.
	 * @param retryAfter - The seconds the site asked to wait, if it said.
	 */
	constructor(retryAfter?: number) {
		super('the site is starting')
		this.retryAfter = retryAfter
	}
}

/** The setup flag the site answers. */
const statusSchema = z.object({ open: z.boolean() })

/** The form a Retry-After header takes when it names seconds. */
const SECONDS = /^\d+$/

/** The refusal a failed setup response carries. */
type Refusal = Awaited<ReturnType<typeof refusalOf>>

const setupRefusals = new Map<number, (refusal: Refusal, response: Response) => Error | undefined>([
	[401, (refusal) => new SetupCodeError(refusal.code === 'setup_code_expired' ? 'expired' : 'invalid')],
	[409, () => new SetupClosedError('the site already has its first account')],
	[422, (refusal) => new ValidationError(refusal.message, refusal)],
	[429, () => new RateLimitedError('too many attempts')],
	[500, (refusal) => (refusal.code === 'setup_code_file' ? new SetupCodeError('file') : undefined)],
	[503, (_, response) => new SiteStartingError(retryAfterOf(response))],
])

/**
 * Returns the seconds a refused response asks to wait.
 * @param response - The refused HTTP response.
 * @returns The seconds, or undefined when its Retry-After header names none.
 */
function retryAfterOf(response: Response): number | undefined {
	const held = response.headers.get('Retry-After')?.trim() ?? ''
	return SECONDS.test(held) ? Number(held) : undefined
}

/**
 * Returns the error a refused setup request throws.
 * @param response - The refused HTTP response.
 * @returns The error to throw.
 */
async function setupRefusal(response: Response): Promise<Error> {
	const refusal = await refusalOf(response, 'the setup request was refused')
	return (
		setupRefusals.get(response.status)?.(refusal, response) ??
		new Error(`the setup request failed with status ${response.status}`)
	)
}

/**
 * Posts a setup request over REST.
 * @param path - The setup route.
 * @param body - The fields the route reads.
 * @returns The accepted response.
 */
async function postSetup(path: string, body: Record<string, string>): Promise<Response> {
	const response = await fetch(path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body),
	})
	if (!response.ok) {
		throw await setupRefusal(response)
	}
	return response
}

/**
 * Reads the setup flag over REST.
 * @param signal - Aborts the in-flight request.
 * @returns True while the site has no account.
 */
async function restIsSetupOpen(signal?: AbortSignal): Promise<boolean> {
	const response = await fetch('/api/auth/setup', { signal })
	if (!response.ok) {
		throw new Error(`loading the setup state failed with status ${response.status}`)
	}
	return statusSchema.parse(await response.json()).open
}

/**
 * Checks the setup code over REST.
 * @param code - The setup code as typed.
 */
async function restCheckSetupCode(code: string): Promise<void> {
	await postSetup('/api/auth/setup/code', { code })
}

/**
 * Creates the first account over REST.
 * @param input - The setup code and the account's details.
 * @returns The created account.
 */
async function restCreateFirstAccount(input: FirstAccount): Promise<Account> {
	const { code, email, name, password } = input
	const response = await postSetup('/api/auth/setup', { code, email, name, password })
	return accountSchema.parse(await response.json())
}

/**
 * Reports whether the site still waits for its first account.
 * @param signal - Aborts the in-flight request.
 * @returns True while the site has no account.
 */
export async function isSetupOpen(signal?: AbortSignal): Promise<boolean> {
	return resolveTransport('isSetupOpen', restIsSetupOpen)(signal)
}

/**
 * Resolves when the site accepts the one-time setup code, spending nothing.
 * @param code - The setup code as typed.
 */
export async function checkSetupCode(code: string): Promise<void> {
	await resolveTransport('checkSetupCode', restCheckSetupCode)(code)
}

/**
 * Creates the site's first account with the one-time setup code.
 * @param input - The setup code and the account's details.
 * @returns The created account.
 */
export async function createFirstAccount(input: FirstAccount): Promise<Account> {
	return resolveTransport('createFirstAccount', restCreateFirstAccount)(input)
}
