// Runs internal/app/testdata/search_cases.json through the search matcher in
// internal/app/static/app.js (the block between the SEARCH-SYNTAX markers).
//
//   node internal/app/testdata/run_search_cases.js
//
// Prints each disagreement and exits 1 on any; exits 0 when every case agrees.
'use strict';
const fs = require('fs');
const path = require('path');

const appPath = path.join(__dirname, '..', 'static', 'app.js');
const source = fs.readFileSync(appPath, 'utf8');
const begin = source.indexOf('// SEARCH-SYNTAX-BEGIN');
const end = source.indexOf('// SEARCH-SYNTAX-END');
if (begin < 0 || end < begin) {
  console.error('the SEARCH-SYNTAX markers were not found in app.js');
  process.exit(2);
}
const block = source.slice(begin, end);
const { parseSearch, searchMatches } = new Function(
  block + '\nreturn { parseSearch: parseSearch, searchMatches: searchMatches };')();

const cases = JSON.parse(fs.readFileSync(path.join(__dirname, 'search_cases.json'), 'utf8'));
let bad = 0;
cases.forEach(function (c, i) {
  const got = searchMatches(parseSearch(c.text), c.members);
  if (got !== c.want) {
    bad++;
    console.log('case ' + i + ': text=' + JSON.stringify(c.text) + ' members=' + JSON.stringify(c.members) +
      ' want=' + c.want + ' got=' + got);
  }
});
console.log(cases.length + ' cases, ' + bad + ' disagreements');
process.exit(bad ? 1 : 0);
