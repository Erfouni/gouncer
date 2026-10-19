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
]

function flagsOf(msgid: string): string[] | undefined {
	const entry = po.parse(readFileSync('languages/es-ES.po', 'utf8')).translations[''][msgid]
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

test("ships WordPress's own words reviewed and ours fuzzy", () => {
	expect(flagsOf('Continue')).toBeDefined()
	expect(flagsOf('Continue')).not.toContain('fuzzy')
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
