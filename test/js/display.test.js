import { describe, it, expect } from 'vitest';
import { fmt1, fmt2, fmtKWh } from '../../cmd/pvs-ui/static/js/display.js';

describe('fmt1', () => {
  it('formats to 1 decimal place', () => expect(fmt1(3.456)).toBe('3.5'));
  it('returns em-dash for null',    () => expect(fmt1(null)).toBe('—'));
  it('returns em-dash for undefined', () => expect(fmt1(undefined)).toBe('—'));
  it('formats zero',                () => expect(fmt1(0)).toBe('0.0'));
  it('formats negative',            () => expect(fmt1(-1.25)).toBe('-1.3'));
});

describe('fmt2', () => {
  it('formats to 2 decimal places', () => expect(fmt2(3.456)).toBe('3.46'));
  it('returns em-dash for null',    () => expect(fmt2(null)).toBe('—'));
  it('formats zero',                () => expect(fmt2(0)).toBe('0.00'));
});

describe('fmtKWh', () => {
  it('formats to 2 decimal places', () => expect(fmtKWh(12.3)).toBe('12.30'));
  it('returns em-dash for null',    () => expect(fmtKWh(null)).toBe('—'));
  it('formats large values',        () => expect(fmtKWh(1234.567)).toBe('1234.57'));
});
