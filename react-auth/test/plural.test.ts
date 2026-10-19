// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { po } from 'gettext-parser'
import { expect, test } from 'vitest'

import { catalogFor } from '../src/languages'

const threeForms = 'nplurals=3; plural=((n == 1) ? 0 : ((n != 0 && n % 1000000 == 0) ? 1 : 2));'

test('declares the three CLDR plural forms in Spanish', async () => {
	const headers = po.parse(readFileSync('languages/es-ES.po', 'utf8')).headers

	expect(headers['Plural-Forms']).toBe(threeForms)
	expect((await catalogFor('es-ES'))?.['']).toMatchObject({ 'plural-forms': threeForms })
})
