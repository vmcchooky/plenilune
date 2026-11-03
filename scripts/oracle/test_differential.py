import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch

import differential as d


class OracleControls(unittest.TestCase):
    def setUp(self):
        any_addr = (0, 0xffffffff)
        self.permit = [('permit', None, any_addr, any_addr, None, None)]
        self.deny = [('deny', None, any_addr, any_addr, None, None)]
        self.packet = {'protocol': 6, 'src_ip': '192.0.2.1', 'dst_ip': '198.51.100.1',
                       'src_port': 65535, 'dst_port': 0}

    def check_with(self, before, after, report):
        with tempfile.TemporaryDirectory() as tmp, patch.object(d, 'run', return_value={'report': report}):
            return d.check(Path('unused'), before, after, Path(tmp))

    def test_wrong_equivalence_fails(self):
        with self.assertRaises(AssertionError):
            self.check_with(self.permit, self.deny, {'verdict': 'EQUIVALENT'})

    def test_inconclusive_is_not_success(self):
        with self.assertRaises(AssertionError):
            self.check_with(self.permit, self.permit, {'verdict': 'INCONCLUSIVE'})

    def test_forged_witness_action_fails(self):
        report = {'verdict': 'DIFFERENT', 'search_complete': True, 'witnesses': [
            {'packet': self.packet, 'before': {'action': 'deny'}, 'after': {'action': 'permit'}}]}
        with self.assertRaises(AssertionError):
            self.check_with(self.permit, self.deny, report)

    def test_unsigned_boundaries(self):
        x = d.z3.BitVecVal(65535, 16)
        self.assertTrue(d.z3.is_true(d.z3.simplify(d.port_match(x, ('gt', 32768)))))
        self.assertTrue(d.z3.is_false(d.z3.simplify(d.port_match(x, ('lt', 1)))))


if __name__ == '__main__':
    unittest.main()
