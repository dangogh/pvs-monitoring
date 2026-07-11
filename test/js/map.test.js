import { describe, it, expect } from 'vitest';
import { normalisePosition, parseCsv } from '../../cmd/pvs-ui/static/js/map.js';

describe('normalisePosition', () => {
  it('strips leading zeros from number',  () => expect(normalisePosition('C02')).toBe('C2'));
  it('leaves non-padded position alone',  () => expect(normalisePosition('B20')).toBe('B20'));
  it('uppercases letter prefix',          () => expect(normalisePosition('c2')).toBe('C2'));
  it('handles single digit',              () => expect(normalisePosition('A1')).toBe('A1'));
  it('handles multi-letter prefix',       () => expect(normalisePosition('AB03')).toBe('AB3'));
  it('returns unchanged if no match',     () => expect(normalisePosition('')).toBe(''));
});

describe('parseCsv', () => {
  const csv = `Position,Serial,Extra
C2,SN001,ignored
B20,SN002,also-ignored
C02,SN003,dup-position
`;

  it('builds positionToSerial map, last row wins on collision', () => {
    const { positionToSerial } = parseCsv(csv);
    // C02 normalises to C2, overwriting the earlier SN001 entry
    expect(positionToSerial['C2']).toBe('SN003');
    expect(positionToSerial['B20']).toBe('SN002');
  });

  it('builds serialToLabel map', () => {
    const { serialToLabel } = parseCsv(csv);
    expect(serialToLabel['SN002']).toBe('B20');
  });

  it('skips blank lines', () => {
    const { positionToSerial } = parseCsv('Position,Serial\n\nA1,SN999\n');
    expect(positionToSerial['A1']).toBe('SN999');
    expect(Object.keys(positionToSerial)).toHaveLength(1);
  });

  it('skips lines with missing fields', () => {
    const { positionToSerial } = parseCsv('Position,Serial\nA1\n');
    expect(Object.keys(positionToSerial)).toHaveLength(0);
  });

  it('returns empty maps for header-only csv', () => {
    const { positionToSerial, serialToLabel } = parseCsv('Position,Serial\n');
    expect(Object.keys(positionToSerial)).toHaveLength(0);
    expect(Object.keys(serialToLabel)).toHaveLength(0);
  });
});
