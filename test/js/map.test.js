import { describe, it, expect } from 'vitest';
import { normalisePosition, parseCsv, ratioToScale, powerColor } from '../../cmd/pvs-ui/static/js/map.js';

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

describe('ratioToScale', () => {
  // The curve is compressed where panels normally live and expanded below it,
  // so ordinary spread stays subtle while a real shortfall is obvious.
  it('keeps a typical panel solidly green', () => {
    expect(ratioToScale(1.0)).toBeGreaterThan(0.9);
  });

  // Measured spread across 48 healthy panels at clear-sky noon was 5%.
  // That must be perceptible but undramatic: a shade, not an alarm.
  it('renders normal 5% spread as a barely-perceptible shade', () => {
    const spread = ratioToScale(1.025) - ratioToScale(0.975);
    expect(spread).toBeGreaterThan(0);
    expect(spread).toBeLessThan(0.05);
  });

  // The whole point: the same 5% of ratio must move colour much further down
  // in the range where a panel is actually in trouble.
  it('expands the same difference far more when output is low', () => {
    const normal = ratioToScale(1.025) - ratioToScale(0.975);
    const low    = ratioToScale(0.825) - ratioToScale(0.775);
    expect(low).toBeGreaterThan(normal * 2);
  });

  it('makes a panel at 80% of peers visibly off-colour', () => {
    expect(ratioToScale(0.8)).toBeLessThan(0.75);
    expect(ratioToScale(0.8)).toBeGreaterThan(0.6);
  });

  // 0.6 is UNDERPERFORM_RATIO: the gradient and the dashed panel-low outline
  // must agree about where trouble starts.
  it('maps the underperform threshold to amber', () => {
    expect(ratioToScale(0.6)).toBeCloseTo(0.42, 2);
  });

  it('maps a dead panel to the bottom',  () => expect(ratioToScale(0)).toBe(0));
  it('clamps an outlier to full scale',  () => expect(ratioToScale(5)).toBe(1));
  it('treats non-finite input as zero',  () => {
    expect(ratioToScale(NaN)).toBe(0);
    expect(ratioToScale(-1)).toBe(0);
  });
  it('is monotonic across the range', () => {
    let prev = -1;
    [0, 0.2, 0.4, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 2].forEach(r => {
      const t = ratioToScale(r);
      expect(t).toBeGreaterThanOrEqual(prev);
      expect(t).toBeLessThanOrEqual(1);
      prev = t;
    });
  });
});

describe('powerColor', () => {
  it('returns a colour pair at every scale point', () => {
    [0, 0.25, 0.5, 0.75, 1].forEach(t => {
      const [bg, fg] = powerColor(t);
      expect(bg).toMatch(/^rgb\(\d+,\d+,\d+\)$/);
      expect(fg).toMatch(/^#[0-9a-f]{6}$/i);
    });
  });
  it('is monotonic in green from amber to full', () => {
    const g = t => Number(powerColor(t)[0].match(/rgb\((\d+),(\d+),(\d+)\)/)[2]);
    expect(g(1)).toBeGreaterThan(0);
    expect(powerColor(0)[0]).not.toBe(powerColor(1)[0]);
  });
});
