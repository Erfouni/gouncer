// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { po } from 'gettext-parser'
import { expect, test } from 'vitest'

import { catalogFor } from '../src/languages'

const oursFuzzy = [
	'Unlock %s',
	'To ensure this site is securely set up by its owner, a setup code has been written to a file on the server.',
	'Please copy the setup code from the file and paste it below.',
	'Setup code',
	'Your setup code appears to be invalid. Please copy it again from the setup code file.',
	'Your setup code has expired. A new one is waiting in the setup code file.',
	'The setup code file cannot be used. Please check it and try again.',
	'You appear to have already set up this site.',
	'The site is starting. Please wait a moment and try again.',
	'Too many attempts. Please try again later.',
	'Sorry, that is not a valid email address. Email addresses look like <code>username@example.com</code>.',
	'Please provide a name.',
	'Hint: The password should be at least %d character long. To make it stronger, use upper and lower case letters, ' +
		'numbers, and symbols like ! " ? $ %% ^ & ).',
	'This name is too long.',
	'This password is too long.',
	'The account could not be created, please try again.',
]

const wordpressReviewed: [context: string, msgid: string][] = [
	['', 'Continue'],
	['', 'Your Email'],
	['', 'Double-check your email address before continuing.'],
	['', 'Important:'],
	['', 'You will need this password to log\u00a0in. Please store it in a secure location.'],
	['', 'Hide'],
	['', 'Show'],
	['', 'Hide password'],
	['', 'Show password'],
	['', 'Caps lock is on.'],
	['password strength', 'Password strength unknown'],
	['password strength', 'Very weak'],
	['password strength', 'Weak'],
	['password strength', 'Medium'],
	['password strength', 'Strong'],
	['', 'Confirm Password'],
	['', 'Confirm use of weak password'],
]

function flagsOf(msgid: string, context = ''): string[] | undefined {
	const entry = po.parse(readFileSync('languages/es-ES.po', 'utf8')).translations[context]?.[msgid]
	return entry && (entry.comments?.flag ?? '').split(',').map((flag) => flag.trim())
}

test('answers the catalogue a locale ships with', async () => {
	const held = await catalogFor('es-ES')

	expect(held).toBeDefined()
	expect(held?.['Log out']).toEqual(['Cerrar sesión'])
})

test('carries the Spanish WordPress uses for a lost password', async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Lost your password?']).toEqual(['¿Has olvidado tu contraseña?'])
})

test("carries WordPress's Spanish for Continue", async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Continue']).toEqual(['Continuar'])
})

test("carries WordPress's Spanish for the account step", async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Your Email']).toEqual(['Tu correo electrónico'])
	expect(held?.['Important:']).toEqual(['Importante:'])
	expect(held?.['Hide password']).toEqual(['Ocultar la contraseña'])
	expect(held?.['Caps lock is on.']).toEqual(['El bloqueo de mayúsculas está activado.'])
})

test("carries WordPress's Spanish for the meter, Medio and never Mediano", async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['password strength\u0004Medium']).toEqual(['Medio'])
	expect(held?.['Confirm use of weak password']).toEqual(['Confirma el uso de una contraseña débil.'])
})

test("ships WordPress's own words reviewed and ours fuzzy", () => {
	for (const [context, msgid] of wordpressReviewed) {
		expect(flagsOf(msgid, context), msgid).toBeDefined()
		expect(flagsOf(msgid, context), msgid).not.toContain('fuzzy')
	}
	for (const msgid of oursFuzzy) {
		expect(flagsOf(msgid), msgid).toContain('fuzzy')
	}
})

test('answers nothing for the language the sources are written in', async () => {
	expect(await catalogFor('en-US')).toBeUndefined()
})

test('answers nothing for a language the package ships no catalogue for', async () => {
	expect(await catalogFor('not-a-locale')).toBeUndefined()
})

test('carries the plural rule the runtime reads', async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['']).toMatchObject({ lang: 'es-ES' })
})
