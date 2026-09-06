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

  describe('peer group column', () => {
    it('reads a column headed "group" into serialToGroup', () => {
      const { serialToGroup } = parseCsv('Position,Serial,Group\nA1,SN001,morning-shaded\n');
      expect(serialToGroup['SN001']).toBe('morning-shaded');
    });

    it('finds the group column wherever it sits', () => {
      const { serialToGroup } = parseCsv(
        'Position,Serial,Lifetime_kWh,Group\nA1,SN001,270.39,unshaded\n');
      expect(serialToGroup['SN001']).toBe('unshaded');
    });

    it('ignores unrelated trailing columns', () => {
      // The deployed map.csv carries a Lifetime_kWh third column; those values
      // must never be mistaken for group names.
      const { serialToGroup } = parseCsv('Position,Serial,Lifetime_kWh\nA1,SN001,270.3954\n');
      expect(serialToGroup).toEqual({});
    });

    it('matches the group header case-insensitively', () => {
      const { serialToGroup } = parseCsv('position,serial,GROUP\nA1,SN001,unshaded\n');
      expect(serialToGroup['SN001']).toBe('unshaded');
    });

    it('leaves serialToGroup empty for a two-column file', () => {
      const { serialToGroup, positionToSerial } = parseCsv('Position,Serial\nA1,SN001\n');
      expect(positionToSerial['A1']).toBe('SN001');   // still parses
      expect(serialToGroup).toEqual({});
    });

    it('omits panels whose group cell is blank', () => {
      const { serialToGroup } = parseCsv('Position,Serial,Group\nA1,SN001,\nA2,SN002,unshaded\n');
      expect(serialToGroup).toEqual({ SN002: 'unshaded' });
    });

    it('trims surrounding whitespace', () => {
      const { serialToGroup } = parseCsv('Position,Serial,Group\nA1,SN001, unshaded \n');
      expect(serialToGroup['SN001']).toBe('unshaded');
    });
  });

  it('returns empty maps for header-only csv', () => {
    const { positionToSerial, serialToLabel } = parseCsv('Position,Serial\n');
    expect(Object.keys(positionToSerial)).toHaveLength(0);
    expect(Object.keys(serialToLabel)).toHaveLength(0);
  });
});
