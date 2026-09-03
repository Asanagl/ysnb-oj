# Builds out2.zip for m4-languages-e2e.mjs: a single test case where the
# expected output echoes the input number (the e2e solutions are echoers).
#   python dbg-mk.py <output-zip-path>
import sys
import zipfile

out = sys.argv[1]
z = zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED)
z.writestr('1.in', '5\n')
z.writestr('1.out', '5\n')
z.close()
