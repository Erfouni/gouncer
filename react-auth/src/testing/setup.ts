// SPDX-License-Identifier: Apache-2.0

import { HttpResponse, http } from 'msw'

import type { Account } from '../account.js'
import type { SetupCodeError } from '../setup.js'

/** createdAccount is the canned first account a setup creates. */
export const createdAccount: Account = {
	id: '0198b2f0-0000-7000-8000-000000000002',
	email: 'maria@example.com',
	name: 'Maria Perez',
	disabled: false,
	role: 'owner',
	created_at: new Date('2026-10-19T10:00:00Z'),
}

/** The answer the site gives for each reason it refuses a setup code. */
const codeRefused: Record<SetupCodeError['reason'], () => Response> = {
	invalid: () => HttpResponse.json({ error: 'setup code invalid', code: 'setup_code_invalid' }, { status: 401 }),
	expired: () => HttpResponse.json({ error: 'setup code expired', code: 'setup_code_expired' }, { status: 401 }),
	file: () => HttpResponse.json({ error: 'setup code file unusable', code: 'setup_code_file' }, { status: 500 }),
}

/**
 * Answers a setup request as already set up.
 * @returns The msw response.
 */
function setupDone() {
	return HttpResponse.json({ error: 'setup already done', code: 'setup_done' }, { status: 409 })
}

/**
 * Answers a setup request as rate limited.
 * @returns The msw response.
 */
function setupThrottled() {
	return HttpResponse.json(
		{ error: 'too many login attempts, try again later', code: 'login_rate_limited' },
		{ status: 429 },
	)
}

/**
 * Answers a setup request as a site that is still starting.
 * @returns The msw response.
 */
function siteStarting() {
	return new HttpResponse(null, { status: 503 })
}

/**
 * Answers the setup state.
 * @param open - Whether the site still waits for its first account.
 * @returns The msw handler.
 */
export function setupStatus(open: boolean) {
	return http.get('/api/auth/setup', () => HttpResponse.json({ open }))
}

/**
 * Accepts a setup code check.
 * @returns The msw handler.
 */
export function setupCodeOk() {
	return http.post('/api/auth/setup/code', () => new HttpResponse(null, { status: 204 }))
}

/**
 * Refuses a setup code check for the given reason.
 * @param reason - Why the site refuses the code.
 * @returns The msw handler.
 */
export function setupCodeRefused(reason: SetupCodeError['reason']) {
	return http.post('/api/auth/setup/code', codeRefused[reason])
}

/**
 * Refuses a setup code check as already set up.
 * @returns The msw handler.
 */
export function setupCodeClosed() {
	return http.post('/api/auth/setup/code', setupDone)
}

/**
 * Refuses a setup code check as rate limited.
 * @returns The msw handler.
 */
export function setupCodeRateLimited() {
	return http.post('/api/auth/setup/code', setupThrottled)
}

/**
 * Answers a setup code check as a site that is still starting.
 * @returns The msw handler.
 */
export function setupCodeStarting() {
	return http.post('/api/auth/setup/code', siteStarting)
}

/**
 * Creates the first account as the given account.
 * @param account - The account the site answers.
 * @returns The msw handler.
 */
export function firstAccountOk(account: Account = createdAccount) {
	return http.post('/api/auth/setup', () => HttpResponse.json(account, { status: 201 }))
}

/**
 * Refuses the first account's setup code for the given reason.
 * @param reason - Why the site refuses the code.
 * @returns The msw handler.
 */
export function firstAccountCodeRefused(reason: SetupCodeError['reason']) {
	return http.post('/api/auth/setup', codeRefused[reason])
}

/**
 * Refuses the first account as already set up.
 * @returns The msw handler.
 */
export function firstAccountClosed() {
	return http.post('/api/auth/setup', setupDone)
}

/**
 * Refuses the first account as rate limited.
 * @returns The msw handler.
 */
export function firstAccountRateLimited() {
	return http.post('/api/auth/setup', setupThrottled)
}

/**
 * Answers the first account as a site that is still starting.
 * @returns The msw handler.
 */
export function firstAccountStarting() {
	return http.post('/api/auth/setup', siteStarting)
}
